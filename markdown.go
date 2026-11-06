package main

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

// newMarkdown builds the shared goldmark converter (CommonMark + GFM).
// Bold/italic always render as <strong>/<em> per the CommonMark spec, never
// the legacy <b>/<i>, so chapters stay uniform. Raw HTML in the source is
// preserved (html.WithUnsafe) rather than escaped or dropped; toXHTML later
// turns it into well-formed XHTML.
func newMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		// Heading ids make "other-chapter.md#some-heading" links resolvable.
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(
			ghtml.WithUnsafe(),
			ghtml.WithXHTML(),
		),
	)
}

// utf8BOM is the byte-order mark some Windows editors prepend to text files.
const utf8BOM = "\xef\xbb\xbf"

// reFrontmatterDelim matches a standalone "---" or "..." line, the
// delimiters YAML frontmatter blocks (e.g. Obsidian notes) start/end with.
var reFrontmatterDelim = regexp.MustCompile(`^(---|\.\.\.)\s*$`)

// stripFrontmatter removes a leading YAML frontmatter block, if any, so it
// never leaks into the rendered chapter. Only strips when the file's very
// first line is a "---" fence and a matching closing fence is found; a
// malformed/unterminated block is left untouched rather than risking
// eating the whole document. A leading UTF-8 BOM and CRLF line endings are
// normalised away first.
func stripFrontmatter(data []byte) []byte {
	text := strings.TrimPrefix(string(data), utf8BOM)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return []byte(text)
	}
	for i := 1; i < len(lines); i++ {
		if reFrontmatterDelim.MatchString(strings.TrimSpace(lines[i])) {
			rest := strings.Join(lines[i+1:], "\n")
			return []byte(strings.TrimLeft(rest, "\n"))
		}
	}
	return []byte(text)
}

// reOrderPrefix strips a leading ordering number some people prefix chapter
// files with, e.g. "01-", "02_", "3. " - so it doesn't leak into the title.
var reOrderPrefix = regexp.MustCompile(`^\d+[-_.\s]+`)

// titleFromFilename turns "03-a-new-beginning.md" into "A New Beginning".
// Used as the chapter title whenever the markdown itself doesn't already
// start with a top-level heading. If nothing is left after stripping the
// order prefix (e.g. "01-.md"), the bare file name is used instead.
func titleFromFilename(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	base := reOrderPrefix.ReplaceAllString(stem, "")
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	words := strings.Fields(base)
	if len(words) == 0 {
		return strings.TrimSpace(stem)
	}
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

var (
	reLeadingH1 = regexp.MustCompile(`(?is)^\s*<h1[^>]*>(.*?)</h1>\s*`)
	reAnyTag    = regexp.MustCompile(`<[^>]+>`)
)

// ensureH1 guarantees the chapter body opens with an <h1>. If goldmark's
// output already starts with one (because the markdown began with "# ..."),
// its text is reused as the chapter title so the visible heading and the
// TOC/nav entry always agree. Otherwise a <h1> built from fallbackTitle is
// prepended.
func ensureH1(bodyHTML, fallbackTitle string) (finalHTML string, title string) {
	if m := reLeadingH1.FindStringSubmatch(bodyHTML); m != nil {
		plain := strings.TrimSpace(html.UnescapeString(reAnyTag.ReplaceAllString(m[1], "")))
		if plain == "" {
			plain = fallbackTitle
		}
		return bodyHTML, plain
	}
	return fmt.Sprintf("<h1>%s</h1>\n%s", html.EscapeString(fallbackTitle), bodyHTML), fallbackTitle
}

// chapter is one rendered .md file.
type chapter struct {
	title    string // used for the visible <h1>, the TOC page, and nav/ncx
	filename string // internal xhtml filename, without extension
	body     string
}

// buildChapters renders every .md file in entries (already in filename
// order, as returned by os.ReadDir) into a chapter. Links between the .md
// files are rewritten to point at the chapter pages they become, and raw HTML
// is normalised to well-formed XHTML.
func buildChapters(md goldmark.Markdown, dir string, entries []os.DirEntry) ([]chapter, error) {
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.ToLower(filepath.Ext(e.Name())) == ".md" {
			names = append(names, e.Name())
		}
	}

	// First pass: decide every chapter's page name, so that links in any
	// chapter can point at any other.
	targets := make(map[string]string, len(names)*2)
	for i, name := range names {
		page := fmt.Sprintf("chapter-%03d", i+1) + ".xhtml"
		if _, taken := targets[name]; !taken {
			targets[name] = page
		}
		if _, taken := targets[strings.ToLower(name)]; !taken {
			targets[strings.ToLower(name)] = page
		}
	}

	chapters := make([]chapter, 0, len(names))
	for i, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		content := stripFrontmatter(raw)

		var buf bytes.Buffer
		if err := md.Convert(content, &buf); err != nil {
			return nil, fmt.Errorf("rendering %s: %w", name, err)
		}

		conv := &xhtmlConverter{targets: targets, where: name}
		xhtml, err := conv.toXHTML(buf.String())
		if err != nil {
			return nil, fmt.Errorf("converting %s to xhtml: %w", name, err)
		}

		body, title := ensureH1(xhtml, titleFromFilename(name))
		chapters = append(chapters, chapter{
			title:    title,
			filename: fmt.Sprintf("chapter-%03d", i+1),
			body:     body,
		})
	}
	return chapters, nil
}
