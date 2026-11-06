package main

import (
	"fmt"
	"html"
	"strings"

	"github.com/vincent-petithory/dataurl"
)

// navEntry is one link on the visible TOC page. href is relative to the
// xhtml/ folder, i.e. just "chapter-001.xhtml" for a sibling file, since
// that's where the TOC page itself lives.
type navEntry struct {
	title string
	href  string
}

// tocPageBody renders the visible "Table of Contents" chapter. Its entries
// mirror what ends up in the EPUB3 nav.xhtml / EPUB2 toc.ncx - the cover
// (if any), the title page, the TOC itself, every chapter, and the back
// cover (if any) - so a reader sees the same outline whether they open the
// TOC page or their app's own navigation menu.
func tocPageBody(entries []navEntry) string {
	var b strings.Builder
	b.WriteString("<h1>Table of Contents</h1>\n<ul>\n")
	for _, e := range entries {
		fmt.Fprintf(&b, `<li><a href="%s">%s</a></li>`+"\n", e.href, html.EscapeString(e.title))
	}
	b.WriteString("</ul>\n")
	return b.String()
}

// Simple, top-aligned title page: everything stacks in normal document
// flow with a fixed gap between blocks.
const titlePageCSS = `body {
	margin: 0;
	padding: 3em 2em;
	box-sizing: border-box;
	font-family: serif;
}
.title-page-title {
	text-align: center;
	font-size: 2.2em;
	font-weight: bold;
	line-height: 1.3;
	margin: 0;
}
.title-page-author {
	text-align: center;
	font-size: 1.3em;
	margin: 0.5em 0 0;
}
.title-page-description {
	text-align: justify;
	margin: 2.5em 0 0;
}
.title-page-publisher {
	text-align: center;
	font-size: 0.9em;
	color: #555555;
	margin: 2.5em 0 0;
}`

// titlePageBody stacks, top to bottom: centered title, centered author, a
// gap, justified description, a gap, then a centered publisher paragraph.
//
// publisherHTML (content.yaml's publisher-info) is inserted as-is as the
// innerHTML of that paragraph - it's expected to already be HTML (e.g. a
// link) and is not escaped. If it's empty, publisherText (content.yaml's
// plain publisher name) is used instead, escaped as plain text.
func titlePageBody(title, author, description, publisherHTML, publisherText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<p class="title-page-title">%s</p>`+"\n", html.EscapeString(title))
	if author != "" {
		fmt.Fprintf(&b, `<p class="title-page-author">%s</p>`+"\n", html.EscapeString(author))
	}
	if description != "" {
		fmt.Fprintf(&b, `<p class="title-page-description">%s</p>`+"\n", html.EscapeString(description))
	}

	publisherInner := publisherHTML
	if publisherInner == "" && publisherText != "" {
		publisherInner = html.EscapeString(publisherText)
	}
	if publisherInner != "" {
		fmt.Fprintf(&b, `<p class="title-page-publisher">%s</p>`+"\n", publisherInner)
	}
	return b.String()
}

const backCoverCSS = `body { margin: 0; padding: 0; text-align: center; }
img { max-width: 100%; max-height: 100%; }`

// cssDataURL encodes CSS content as a data URL, since AddCSS expects a
// source it can fetch (URL, local path, or data URL) rather than raw text.
func cssDataURL(css string) string {
	return dataurl.EncodeBytes([]byte(css))
}
