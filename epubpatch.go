package main

// Post-write patching of the generated EPUB.
//
// go-epub's SetCover deliberately excludes the cover page from the
// generated nav.xhtml/toc.ncx (its own doc comment says so), because a
// reader's cover is normally handled through dedicated EPUB cover metadata,
// not the chapter list. To also make it a normal, clickable TOC/nav entry,
// this patches those two generated files after the fact rather than fight
// the library's internal (and unexported) cover bookkeeping. The same pass
// also writes the optional publication date into the package (.opf)
// metadata, which the library has no setter for.
//
// The cover patch relies on SetCover's cover page always landing at
// "xhtml/cover.xhtml" - true as long as it's the very first section added
// (which buildBook ensures) so no filename collision forces the library to
// pick a fallback name.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
)

// patchOptions says what postProcessEpub should change. Zero values mean
// "leave that part alone".
type patchOptions struct {
	coverHref  string // e.g. "xhtml/cover.xhtml"; empty = no cover entry
	coverTitle string
	date       string // dc:date value; empty = none
}

var (
	reNavOlOpen     = regexp.MustCompile(`<ol[^>]*>`)
	reNcxNavMapOpen = regexp.MustCompile(`<navMap[^>]*>`)
	reOpfMetaClose  = regexp.MustCompile(`</(?:\w+:)?metadata>`)
)

func insertNavListItem(xhtml, href, title string) (string, error) {
	loc := reNavOlOpen.FindStringIndex(xhtml)
	if loc == nil {
		return "", fmt.Errorf("could not find <ol> in nav.xhtml")
	}
	insertion := fmt.Sprintf("\n        <li>\n          <a href=\"%s\">%s</a>\n        </li>", href, html.EscapeString(title))
	return xhtml[:loc[1]] + insertion + xhtml[loc[1]:], nil
}

func insertNcxNavPoint(ncx, href, title string) (string, error) {
	loc := reNcxNavMapOpen.FindStringIndex(ncx)
	if loc == nil {
		return "", fmt.Errorf("could not find <navMap> in toc.ncx")
	}
	insertion := fmt.Sprintf("\n    <navPoint id=\"navPoint-cover\">\n      <navLabel>\n        <text>%s</text>\n      </navLabel>\n      <content src=\"%s\"></content>\n    </navPoint>", html.EscapeString(title), href)
	return ncx[:loc[1]] + insertion + ncx[loc[1]:], nil
}

func insertOpfDate(opf, date string) (string, error) {
	if strings.Contains(opf, "<dc:date") {
		return opf, nil // already has one; don't duplicate
	}
	loc := reOpfMetaClose.FindStringIndex(opf)
	if loc == nil {
		return "", fmt.Errorf("could not find </metadata> in package file")
	}
	insertion := fmt.Sprintf("    <dc:date>%s</dc:date>\n  ", html.EscapeString(date))
	return opf[:loc[0]] + insertion + opf[loc[0]:], nil
}

// postProcessEpub rewrites the in-memory EPUB zip, applying opts. Every
// untouched file is copied through byte-for-byte (via raw zip entries) so
// nothing else about the archive - including the mandatory uncompressed
// "mimetype" file being first - changes.
func postProcessEpub(epubBytes []byte, opts patchOptions) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(epubBytes), int64(len(epubBytes)))
	if err != nil {
		return nil, fmt.Errorf("reading generated epub: %w", err)
	}

	var out bytes.Buffer
	zw := zip.NewWriter(&out)

	// patch rewrites one entry's text through fn.
	patch := func(f *zip.File, fn func(string) (string, error)) error {
		data, err := readZipEntry(f)
		if err != nil {
			return err
		}
		patched, err := fn(string(data))
		if err != nil {
			return fmt.Errorf("patching %s: %w", f.Name, err)
		}
		return writeZipEntry(zw, f.Name, []byte(patched))
	}

	for _, f := range zr.File {
		var err error
		switch {
		case opts.coverHref != "" && f.Name == "EPUB/nav.xhtml":
			err = patch(f, func(s string) (string, error) {
				return insertNavListItem(s, opts.coverHref, opts.coverTitle)
			})
		case opts.coverHref != "" && f.Name == "EPUB/toc.ncx":
			err = patch(f, func(s string) (string, error) {
				return insertNcxNavPoint(s, opts.coverHref, opts.coverTitle)
			})
		case opts.date != "" && strings.HasPrefix(f.Name, "EPUB/") && strings.HasSuffix(f.Name, ".opf"):
			err = patch(f, func(s string) (string, error) {
				return insertOpfDate(s, opts.date)
			})
		default:
			if cerr := copyZipEntryRaw(zw, f); cerr != nil {
				err = fmt.Errorf("copying %s: %w", f.Name, cerr)
			}
		}
		if err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func writeZipEntry(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// copyZipEntryRaw copies a zip entry through unmodified, preserving its
// original compression method and bytes exactly - used for every file we
// don't need to touch, most importantly the EPUB spec's mandatory
// uncompressed, first-in-archive "mimetype" file.
func copyZipEntryRaw(zw *zip.Writer, f *zip.File) error {
	rc, err := f.OpenRaw()
	if err != nil {
		return err
	}
	hdr := f.FileHeader
	w, err := zw.CreateRaw(&hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, rc)
	return err
}
