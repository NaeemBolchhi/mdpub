package main

// Raw HTML in Markdown is passed through goldmark untouched, and HTML is far
// more forgiving than the XML EPUB readers (and EPUBCheck) require. This file
// re-parses each chapter body with a real HTML5 parser and re-serialises it
// as well-formed XHTML, fixing the common problems on the way:
//
//   - unclosed / misnested tags and void elements (<br>, <img ...>, <hr>)
//   - boolean and unquoted attributes, stray entities such as &nbsp;
//   - "--" inside comments
//   - SVG / MathML without their XML namespace declarations
//   - elements and attributes that are never valid in an EPUB content
//     document (script, iframe, style-in-body, on* handlers, ...), which are
//     dropped with a warning
//   - obsolete presentational markup that goldmark itself emits for tables
//     (align="...") or that is common in hand-written HTML (<center>)
//   - <img> without alt, which EPUBCheck rejects
//
// It also rewrites links between chapters (see rewriteChapterLink).

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	nsSVG    = "http://www.w3.org/2000/svg"
	nsMathML = "http://www.w3.org/1998/Math/MathML"
	nsXLink  = "http://www.w3.org/1999/xlink"
)

// droppedElements can't appear in an EPUB chapter body (or need manifest
// support this tool doesn't provide, like scripting). Only applies to
// elements in the HTML namespace; <style> inside an <svg> is legal.
var droppedElements = map[string]string{
	"script":    "scripts are not supported",
	"noscript":  "scripts are not supported",
	"iframe":    "iframes are not allowed",
	"embed":     "embedded objects are not supported",
	"object":    "embedded objects are not supported",
	"xmp":       "obsolete element",
	"noembed":   "obsolete element",
	"noframes":  "obsolete element",
	"plaintext": "obsolete element",
	"style":     "<style> is not allowed inside the body; use styles.css instead",
	"link":      "<link> is not allowed inside the body; use styles.css instead",
	"meta":      "<meta> is not allowed inside the body",
	"base":      "<base> is not allowed inside the body",
	"title":     "<title> is not allowed inside the body",
}

// reXMLName is the attribute-name shape we're willing to emit; anything else
// (e.g. Vue's "@click", or ":class") isn't well-formed XML.
var reXMLName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*(:[A-Za-z_][A-Za-z0-9_.\-]*)?$`)

type xhtmlConverter struct {
	// targets maps source .md file names to their chapter xhtml file names
	// for link rewriting; nil disables it.
	targets map[string]string
	// where is used in warnings, e.g. the source file name.
	where  string
	warned map[string]bool
}

func (c *xhtmlConverter) warnf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if c.warned[msg] {
		return
	}
	if c.warned == nil {
		c.warned = map[string]bool{}
	}
	c.warned[msg] = true
	warnf("%s: %s", c.where, msg)
}

// toXHTML converts an HTML fragment (typically goldmark's output) into
// well-formed XHTML.
func (c *xhtmlConverter) toXHTML(fragment string) (string, error) {
	ctx := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := html.ParseFragmentWithOptions(strings.NewReader(fragment), ctx,
		html.ParseOptionEnableScripting(false))
	if err != nil {
		return "", fmt.Errorf("parsing html: %w", err)
	}
	root := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	c.walk(root)

	var buf bytes.Buffer
	for ch := root.FirstChild; ch != nil; ch = ch.NextSibling {
		if err := html.Render(&buf, ch); err != nil {
			return "", fmt.Errorf("rendering html: %w", err)
		}
	}
	return buf.String(), nil
}

func (c *xhtmlConverter) walk(n *html.Node) {
	for ch := n.FirstChild; ch != nil; {
		next := ch.NextSibling
		switch ch.Type {
		case html.CommentNode:
			ch.Data = sanitizeComment(ch.Data)
		case html.ElementNode:
			if ch.Namespace == "" {
				if why, drop := droppedElements[ch.Data]; drop {
					c.warnf("dropped <%s>: %s", ch.Data, why)
					n.RemoveChild(ch)
					ch = next
					continue
				}
			}
			c.fixElement(ch)
			c.walk(ch)
		}
		ch = next
	}
}

// sanitizeComment makes comment text legal in XML: no "--" and no trailing "-".
func sanitizeComment(s string) string {
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "- -")
	}
	if strings.HasSuffix(s, "-") {
		s += " "
	}
	return s
}

func (c *xhtmlConverter) fixElement(n *html.Node) {
	if n.Namespace == "" {
		switch n.Data {
		case "center":
			n.Data, n.DataAtom = "div", atom.Div
			addStyle(n, "text-align:center")
			c.warnf("converted obsolete <center> to a <div>")
		case "td", "th":
			if v, ok := takeAttr(n, "align"); ok {
				if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
					addStyle(n, "text-align:"+v)
				}
			}
		case "img":
			if !hasAttr(n, "alt") {
				n.Attr = append(n.Attr, html.Attribute{Key: "alt", Val: ""})
			}
		case "a":
			c.rewriteLink(n)
		}
	}

	// Foreign (SVG / MathML) roots need their XML namespace declared, since
	// the HTML parser has no way to carry xmlns through serialisation.
	if n.Namespace == "svg" || n.Namespace == "math" {
		if n.Parent == nil || n.Parent.Namespace != n.Namespace {
			want := nsSVG
			if n.Namespace == "math" {
				want = nsMathML
			}
			if !hasAttr(n, "xmlns") {
				n.Attr = append(n.Attr, html.Attribute{Key: "xmlns", Val: want})
			}
			if usesXLink(n) && !hasAttrKey(n, "xmlns:xlink") {
				n.Attr = append(n.Attr, html.Attribute{Key: "xmlns:xlink", Val: nsXLink})
			}
		}
	}

	kept := n.Attr[:0]
	for _, a := range n.Attr {
		key := a.Key
		if a.Namespace != "" {
			key = a.Namespace + ":" + a.Key
		}
		switch {
		case strings.HasPrefix(strings.ToLower(key), "on") && a.Namespace == "":
			c.warnf("dropped event handler attribute %q", key)
			continue
		case !reXMLName.MatchString(key):
			c.warnf("dropped invalid attribute name %q", key)
			continue
		}
		kept = append(kept, a)
	}
	n.Attr = kept
}

func usesXLink(n *html.Node) bool {
	for _, a := range n.Attr {
		if a.Namespace == "xlink" {
			return true
		}
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && usesXLink(ch) {
			return true
		}
	}
	return false
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return true
		}
	}
	return false
}

func hasAttrKey(n *html.Node, full string) bool {
	for _, a := range n.Attr {
		k := a.Key
		if a.Namespace != "" {
			k = a.Namespace + ":" + a.Key
		}
		if k == full {
			return true
		}
	}
	return false
}

// takeAttr removes and returns an (un-namespaced) attribute.
func takeAttr(n *html.Node, key string) (string, bool) {
	for i, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			return a.Val, true
		}
	}
	return "", false
}

// addStyle appends a declaration to the element's style attribute.
func addStyle(n *html.Node, decl string) {
	for i, a := range n.Attr {
		if a.Namespace == "" && a.Key == "style" {
			v := strings.TrimSpace(a.Val)
			if v != "" && !strings.HasSuffix(v, ";") {
				v += ";"
			}
			n.Attr[i].Val = strings.TrimSpace(v + decl)
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: "style", Val: decl})
}

// ---------- links between chapters ----------

// rewriteChapterLink maps a Markdown-style link to another source file, such
// as "02-next.md" or "../book/02-next.md#part-2", to the chapter page it
// became, e.g. "chapter-002.xhtml#part-2". isMD reports whether href looked
// like a link to a .md file at all (so the caller can warn when no matching
// chapter exists). External URLs, absolute paths and bare "#fragment" links
// are never touched.
func rewriteChapterLink(href string, targets map[string]string) (newHref string, rewritten, isMD bool) {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") {
		return href, false, false
	}
	if strings.ToLower(path.Ext(u.Path)) != ".md" {
		return href, false, false
	}
	p := path.Clean(u.Path) // u.Path is already percent-decoded
	target, ok := targets[p]
	if !ok {
		target, ok = targets[strings.ToLower(p)]
	}
	if !ok { // be lenient about folder prefixes, e.g. Obsidian-style paths
		base := path.Base(p)
		if target, ok = targets[base]; !ok {
			target, ok = targets[strings.ToLower(base)]
		}
	}
	if !ok {
		return href, false, true
	}
	if frag := u.EscapedFragment(); frag != "" {
		target += "#" + frag
	}
	return target, true, true
}

func (c *xhtmlConverter) rewriteLink(n *html.Node) {
	if c.targets == nil {
		return
	}
	for i, a := range n.Attr {
		if a.Namespace != "" || a.Key != "href" {
			continue
		}
		newHref, rewritten, isMD := rewriteChapterLink(a.Val, c.targets)
		switch {
		case rewritten:
			n.Attr[i].Val = newHref
		case isMD:
			c.warnf("link to %q doesn't match any .md file in the book; left unchanged", a.Val)
		}
	}
}
