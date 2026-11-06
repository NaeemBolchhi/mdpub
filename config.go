package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// bookMeta holds the flat set of metadata fields content.yaml may define.
type bookMeta struct {
	Title         string
	Author        string
	Language      string
	Identifier    string
	Description   string
	Publisher     string
	PublisherInfo string
	Date          string
}

// unquote strips one pair of matching surrounding quotes, and nothing else,
// so a value like  Authors'  keeps its apostrophe.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// loadContentYAML reads a minimal "key: value" per line subset of YAML -
// exactly as much as flat book metadata needs. It intentionally isn't a
// general YAML parser: content.yaml is expected to be a flat list of
// fields on single lines, not nested structures or block scalars.
func loadContentYAML(path string) (bookMeta, error) {
	var meta bookMeta
	data, err := os.ReadFile(path)
	if err != nil {
		return meta, err
	}
	fields := map[string]*string{
		"title":          &meta.Title,
		"author":         &meta.Author,
		"language":       &meta.Language,
		"lang":           &meta.Language,
		"identifier":     &meta.Identifier,
		"isbn":           &meta.Identifier,
		"uuid":           &meta.Identifier,
		"description":    &meta.Description,
		"publisher":      &meta.Publisher,
		"publisher-info": &meta.PublisherInfo,
		"date":           &meta.Date,
	}
	text := strings.TrimPrefix(string(data), utf8BOM)
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx == -1 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		val := unquote(strings.TrimSpace(line[idx+1:]))
		if dst, ok := fields[key]; ok && val != "" {
			*dst = val
		}
	}
	return meta, nil
}

// reDCDate matches the date forms EPUB's dc:date accepts: YYYY, YYYY-MM,
// YYYY-MM-DD, or a full UTC timestamp.
var reDCDate = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2}(T\d{2}:\d{2}:\d{2}Z)?)?)?$`)

// contentYAMLTemplate is the starter file written by -config.
const contentYAMLTemplate = `# Book metadata for mdpub. Every field is optional - omit whatever you
# don't need. Flat key: value only - no nesting, no lists, no multi-line
# block scalars (folded/literal style) - keep every value on one line.

title: 
author: 
language: en

# Any unique string works: a UUID, ISBN, DOI, or your own scheme. Leave
# blank to have one generated automatically.
identifier: 

# Also shown on the title page, justified, between the author and the
# publisher line.
description: 

# Plain-text fallback for the title page's publisher line - only used if
# publisher-info (below) isn't set.
publisher: 

# Raw HTML, inserted as-is (not escaped) as the publisher paragraph on the
# title page. Takes priority over the plain "publisher" field above if
# both are set.
publisher-info: 

# Publication date, written into the book's metadata: YYYY, YYYY-MM or
# YYYY-MM-DD.
date: 
`

// scaffoldContentYAML writes a template content.yaml into dir (creating
// dir if it doesn't exist yet), refusing to overwrite a content.yaml that
// already exists.
func scaffoldContentYAML(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %q: %w", dir, err)
	}
	path := filepath.Join(dir, "content.yaml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; remove it first if you want a fresh template", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(contentYAMLTemplate), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Printf("wrote %s\n", path)
	return nil
}
