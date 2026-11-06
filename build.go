package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	epub "github.com/go-shiori/go-epub"
)

// warnf prints a non-fatal problem to stderr.
func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", args...)
}

// bookTitle picks the book title: content.yaml's title, else a title derived
// from the source directory's name. The path is made absolute first so that
// "." (the default) yields the real folder name rather than ".".
func bookTitle(meta bookMeta, dir string) string {
	if meta.Title != "" {
		return meta.Title
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if title := titleFromFilename(filepath.Base(filepath.Clean(dir))); title != "" {
		return title
	}
	return "Book"
}

// run converts the Markdown files in dir into a single EPUB written to out
// (see resolveOutputPath for how out is interpreted).
func run(dir, out string) error {
	dir, err := resolveDir(dir)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cannot read %q: %w", dir, err)
	}

	meta, err := loadContentYAML(filepath.Join(dir, "content.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading content.yaml: %w", err)
	}
	title := bookTitle(meta, dir)

	e, err := epub.NewEpub(title)
	if err != nil {
		return fmt.Errorf("creating epub: %w", err)
	}
	if meta.Author != "" {
		e.SetAuthor(meta.Author)
	}
	e.SetLang("en")
	if meta.Language != "" {
		e.SetLang(meta.Language)
	}
	if meta.Identifier != "" {
		e.SetIdentifier(meta.Identifier)
	}
	if meta.Description != "" {
		e.SetDescription(meta.Description)
	}

	// Front cover (optional): "cover" takes priority over "frontcover" if
	// both exist. Must be set before any other AddSection call so its
	// generated page is guaranteed to land at the default "cover.xhtml"
	// filename the post-write nav/ncx patch assumes.
	hasCover := false
	if imgPath, ok := findNamedImage(entries, dir, "cover", "frontcover"); ok {
		internalImg, err := e.AddImage(imgPath, "")
		if err != nil {
			return fmt.Errorf("adding cover image: %w", err)
		}
		if err := e.SetCover(internalImg, ""); err != nil {
			return fmt.Errorf("setting cover: %w", err)
		}
		hasCover = true
	}

	// Back cover (optional): detected now so it can be listed on the TOC
	// page below; actually added to the book after the chapters, so it
	// lands last in reading/nav order.
	backCoverPath, hasBackCover := findNamedImage(entries, dir, "backcover", "rearcover")

	// Every other image file alongside the .md files gets copied into the
	// epub's images folder as-is, unreferenced, for the user to link to
	// manually later.
	if err := addLooseImages(e, entries, dir); err != nil {
		return err
	}

	// Optional shared stylesheet, embedded and linked into the chapters and
	// the TOC page if present alongside the .md files.
	var contentCSS string
	if _, err := os.Stat(filepath.Join(dir, "styles.css")); err == nil {
		internalCSS, err := e.AddCSS(filepath.Join(dir, "styles.css"), "styles.css")
		if err != nil {
			return fmt.Errorf("adding styles.css: %w", err)
		}
		contentCSS = internalCSS
	}

	chapters, err := buildChapters(newMarkdown(), dir, entries)
	if err != nil {
		return err
	}
	if len(chapters) == 0 {
		return fmt.Errorf("no .md files found in %q", dir)
	}

	// The visible TOC page's entries mirror the nav/ncx exactly: cover,
	// title page, the TOC itself, every chapter, back cover.
	var tocEntries []navEntry
	if hasCover {
		tocEntries = append(tocEntries, navEntry{"Cover", "cover.xhtml"})
	}
	tocEntries = append(tocEntries,
		navEntry{"Title Page", "titlepage.xhtml"},
		navEntry{"Table of Contents", "toc.xhtml"},
	)
	for _, c := range chapters {
		tocEntries = append(tocEntries, navEntry{c.title, c.filename + ".xhtml"})
	}
	if hasBackCover {
		tocEntries = append(tocEntries, navEntry{"Back Cover", "backcover.xhtml"})
	}

	// Title page: added right after the cover, before the TOC.
	titlePageInternalCSS, err := e.AddCSS(cssDataURL(titlePageCSS), "titlepage.css")
	if err != nil {
		return fmt.Errorf("adding title page css: %w", err)
	}
	// publisher-info is raw HTML, so it gets the same XHTML clean-up as chapters.
	publisherInfo := meta.PublisherInfo
	if publisherInfo != "" {
		conv := &xhtmlConverter{where: "content.yaml (publisher-info)"}
		if fixed, err := conv.toXHTML(publisherInfo); err != nil {
			warnf("content.yaml: could not parse publisher-info as HTML (%v); ignoring it", err)
			publisherInfo = ""
		} else {
			publisherInfo = fixed
		}
	}
	titleBody := titlePageBody(title, meta.Author, meta.Description, publisherInfo, meta.Publisher)
	if _, err := e.AddSection(titleBody, "Title Page", "titlepage", titlePageInternalCSS); err != nil {
		return fmt.Errorf("adding title page: %w", err)
	}

	if _, err := e.AddSection(tocPageBody(tocEntries), "Table of Contents", "toc", contentCSS); err != nil {
		return fmt.Errorf("adding table of contents: %w", err)
	}
	for _, c := range chapters {
		if _, err := e.AddSection(c.body, c.title, c.filename, contentCSS); err != nil {
			return fmt.Errorf("adding chapter %q: %w", c.title, err)
		}
	}

	if hasBackCover {
		internalImg, err := e.AddImage(backCoverPath, "")
		if err != nil {
			return fmt.Errorf("adding back cover image: %w", err)
		}
		internalCSS, err := e.AddCSS(cssDataURL(backCoverCSS), "backcover.css")
		if err != nil {
			return fmt.Errorf("adding back cover css: %w", err)
		}
		body := fmt.Sprintf(`<img src="%s" alt="Back Cover" />`, internalImg)
		if _, err := e.AddSection(body, "Back Cover", "backcover", internalCSS); err != nil {
			return fmt.Errorf("adding back cover: %w", err)
		}
	}

	var buf bytes.Buffer
	if _, err := e.WriteTo(&buf); err != nil {
		return fmt.Errorf("writing epub: %w", err)
	}
	finalBytes := buf.Bytes()

	opts := patchOptions{}
	if hasCover {
		opts.coverHref, opts.coverTitle = "xhtml/cover.xhtml", "Cover"
	}
	if meta.Date != "" {
		if reDCDate.MatchString(meta.Date) {
			opts.date = meta.Date
		} else {
			warnf("ignoring date %q (use YYYY, YYYY-MM or YYYY-MM-DD)", meta.Date)
		}
	}
	if opts.coverHref != "" || opts.date != "" {
		finalBytes, err = postProcessEpub(finalBytes, opts)
		if err != nil {
			return fmt.Errorf("post-processing epub: %w", err)
		}
	}

	finalOutPath, err := resolveOutputPath(out, title)
	if err != nil {
		return err
	}
	if err := os.WriteFile(finalOutPath, finalBytes, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", finalOutPath, err)
	}
	fmt.Printf("wrote %s (%d chapters)\n", finalOutPath, len(chapters))
	return nil
}
