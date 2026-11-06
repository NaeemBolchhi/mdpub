package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// resolveDir validates path and, if it turns out to point at a regular
// file rather than a directory (e.g. someone passed -dir the path of one
// of their .md files by mistake), quietly falls back to that file's parent
// directory instead of erroring.
func resolveDir(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot access %q: %w", path, err)
	}
	if !info.IsDir() {
		path = filepath.Dir(path)
	}
	return path, nil
}

// resolveOutputDir figures out a plain directory from a lenient
// output-location value: empty means the current directory; an existing
// directory is used as-is; an existing file's parent directory is used; a
// path that doesn't exist yet but looks like a file (has an extension) has
// its parent directory used; anything else is treated as a directory to be
// created.
func resolveOutputDir(raw string) (string, error) {
	if raw == "" {
		return ".", nil
	}
	if info, err := os.Stat(raw); err == nil {
		if info.IsDir() {
			return raw, nil
		}
		return filepath.Dir(raw), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("checking %q: %w", raw, err)
	}
	if filepath.Ext(raw) != "" {
		return filepath.Dir(raw), nil
	}
	return raw, nil
}

// reUnsafeFilenameChars matches characters that are illegal in Windows file
// names (and control characters), which would make a title such as
// "Q&A: Why?" unusable as a file name.
var reUnsafeFilenameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)

// epubFilename turns a book title into a safe "<title>.epub" file name.
func epubFilename(title string) string {
	name := reUnsafeFilenameChars.ReplaceAllString(title, "_")
	name = strings.Trim(name, ". ")
	if name == "" {
		name = "book"
	}
	return name + ".epub"
}

// resolveOutputPath figures out the exact .epub path to write, given
// -out/-outdir/-o's raw combined value (they're all the same flag - see
// main): empty means "<title>.epub" in the current directory; an existing
// directory (or a path that doesn't exist yet and has no file extension)
// means "<title>.epub" inside it, creating the directory first if needed;
// anything that is or looks like a specific file path (an existing file,
// or a nonexistent path with an extension like ".epub") is used exactly as
// given, as the literal target file.
func resolveOutputPath(raw, title string) (string, error) {
	filename := epubFilename(title)

	if raw == "" {
		return filename, nil
	}
	if info, err := os.Stat(raw); err == nil {
		if info.IsDir() {
			return filepath.Join(raw, filename), nil
		}
		return raw, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("checking %q: %w", raw, err)
	}
	if filepath.Ext(raw) != "" {
		if dir := filepath.Dir(raw); dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return "", fmt.Errorf("creating %q: %w", dir, err)
			}
		}
		return raw, nil
	}
	if err := os.MkdirAll(raw, 0755); err != nil {
		return "", fmt.Errorf("creating %q: %w", raw, err)
	}
	return filepath.Join(raw, filename), nil
}
