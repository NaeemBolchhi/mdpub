package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	epub "github.com/go-shiori/go-epub"
)

// coverImageExt are the formats usable as a front/back cover.
var coverImageExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true,
}

// looseImageExt covers every image type bundled as a loose image - both
// raster and vector.
var looseImageExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true, ".svg": true,
}

// reservedImageBaseNames are the filenames (extension stripped,
// case-insensitive) already handled by dedicated logic (cover, back cover).
// Every usable cover-format file matching one of these - not just the single
// one that ends up used - is skipped as a loose image, so e.g. a leftover
// cover.jpg next to the cover.png that actually got used doesn't also get
// dumped into the images folder as an extra, unreferenced "cover".
var reservedImageBaseNames = map[string]bool{
	"cover": true, "frontcover": true, "backcover": true, "rearcover": true,
}

func baseNameLower(name string) string {
	return strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
}

// findNamedImage looks for an image file whose base name (extension
// stripped, case-insensitive) matches one of candidateNames, tried in
// order - so the first name in the list wins if multiple exist (e.g.
// "backcover" beats "rearcover"). If several files share the matched base
// name with different extensions, a .png is preferred; otherwise the first
// match (sorted) is used.
func findNamedImage(entries []os.DirEntry, dir string, candidateNames ...string) (string, bool) {
	grouped := map[string][]string{}
	for _, e := range entries {
		if e.IsDir() || !coverImageExt[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		base := baseNameLower(e.Name())
		grouped[base] = append(grouped[base], e.Name())
	}
	for _, name := range candidateNames {
		files := grouped[strings.ToLower(name)]
		if len(files) == 0 {
			continue
		}
		sort.Strings(files)
		for _, f := range files {
			if strings.ToLower(filepath.Ext(f)) == ".png" {
				return filepath.Join(dir, f), true
			}
		}
		return filepath.Join(dir, files[0]), true
	}
	return "", false
}

// addLooseImages copies every other image file in dir into the epub's
// images/ folder via AddImage, without referencing it from any page, spine
// entry, or nav/toc - purely so it's sitting inside the book for the user
// to link to by hand later (e.g. an <img> tag typed directly into a .md
// file's raw HTML). The original filename is kept as the internal name so
// what the user sees on disk is what they'll reference.
func addLooseImages(e *epub.Epub, entries []os.DirEntry, dir string) error {
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !looseImageExt[ext] {
			continue
		}
		// A cover.svg can't be used as a cover (see coverImageExt), so it is
		// bundled like any other loose image instead of silently vanishing.
		if coverImageExt[ext] && reservedImageBaseNames[baseNameLower(entry.Name())] {
			continue
		}
		if _, err := e.AddImage(filepath.Join(dir, entry.Name()), entry.Name()); err != nil {
			return fmt.Errorf("adding image %s: %w", entry.Name(), err)
		}
	}
	return nil
}
