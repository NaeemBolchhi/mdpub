# mdpub

A small Go CLI that turns a directory of Markdown files into a single, well-formed EPUB — cover, title page, table of contents, and all.

No servers, no config beyond one optional YAML file, no loose intermediate files. Point it at a folder of `.md` files and get one `.epub` out.

This tool was made with **Claude**.

## Features

- **CommonMark + GFM** rendering via [goldmark](https://github.com/yuin/goldmark) — tables, strikethrough, autolinks, task lists included.
- **Raw HTML passthrough, made XHTML-safe** — any HTML written directly in a `.md` file is kept, then re-parsed and re-serialised as well-formed XHTML so EPUBCheck doesn't choke on `<br>`, unclosed tags, `&nbsp;`, bare attributes, `align=` on table cells, `<center>`, `<img>` without `alt`, SVG without a namespace, and so on. Things that can never be valid in a chapter body (`<script>`, `<iframe>`, `<style>`, `on*` handlers, ...) are dropped with a warning on stderr.
- **Links between chapters** — `[next](02-next.md)` and `[x](02-next.md#some-heading)` are rewritten to the chapter pages they become. Headings get automatic ids, so `#fragment` links resolve. A link to a `.md` file that isn't in the book is left alone with a warning.
- **YAML frontmatter stripping** — a leading `---` block (Obsidian-style) is removed before rendering.
- **Front cover** — looked up as `cover.*` or `frontcover.*` (first name wins if both exist); `.png` preferred if multiple extensions exist.
- **Back cover** — looked up as `backcover.*` or `rearcover.*`, same preference rules.
- **Loose image bundling** — every other image file (raster or SVG) next to your `.md` files is copied into the EPUB's image folder, unreferenced, so you can hand-link them from your Markdown later. Chapters live in `xhtml/` and images in `images/`, so reference them as `../images/diagram.png`.
- **Title page** — book title, author, description, and publisher, generated automatically from `content.yaml`.
- **Table of contents** — a real, visible TOC page, plus a matching EPUB3 `nav.xhtml` and EPUB2 `toc.ncx` so reading apps show the same outline in their own menus. The front cover is included in all of these too, not just the reading order.
- **Optional shared stylesheet** — a `styles.css` next to your `.md` files is embedded and linked into every chapter and the TOC page.
- **Config scaffolding** — `mdpub.exe -config` writes a starter `content.yaml` for you.
- **Flexible input/output flags**, including tolerance for being handed a file path instead of a directory.
- Validated against [EPUBCheck](https://github.com/w3c/epubcheck) (EPUB 3.2) throughout development.

## Install / build

Requires Go 1.22+.

```sh
go build -o mdpub.exe .
```

## Usage

```sh
# Convert the current directory into <title-or-dirname>.epub, written here
mdpub.exe

# Convert a specific directory
mdpub.exe -dir ./my-book

# Choose where the .epub goes - a directory (filename auto-derived)...
mdpub.exe -dir ./my-book -out ./dist

# ...or a specific file path
mdpub.exe -dir ./my-book -out ./dist/final.epub

# Scaffold a starter content.yaml
mdpub.exe -config -dir ./my-book
```

### Flags

| Flag | Aliases | Meaning |
|---|---|---|
| `-dir` | `-in`, `-indir`, `-i` | Directory containing the `.md` files. Also accepts a file path inside that directory — its parent is used. Defaults to the current directory. |
| `-out` | `-outdir`, `-o` | Where to write the `.epub`. A directory (created if needed) gets `<title>.epub` inside it; a specific file path is used exactly as given. Defaults to `<title>.epub` in the current directory. |
| `-config` | | Writes a starter `content.yaml` and exits (doesn't build an epub). Uses whichever of `-dir`/`-out` is given, preferring `-dir` if both are, falling back to the current directory if neither is. Refuses to overwrite an existing `content.yaml`. |

Characters that are illegal in file names (`: ? / \ *` and friends) are replaced with `_` in the generated `<title>.epub` name.

## Source layout

| File | Responsibility |
|---|---|
| `main.go` | flag parsing, `-config` mode |
| `build.go` | `run`: assembles the book end to end |
| `config.go` | `content.yaml` parsing and scaffolding |
| `markdown.go` | goldmark setup, frontmatter, chapter titles |
| `xhtml.go` | raw HTML → XHTML clean-up, chapter link rewriting |
| `images.go` | cover / back cover / loose image discovery |
| `pages.go` | title page, TOC page, back cover markup |
| `epubpatch.go` | post-write patch of cover nav entry and `dc:date` |
| `paths.go` | input/output path resolution |

## Directory layout

```
my-book/
├── content.yaml        # optional metadata (see below)
├── styles.css          # optional, embedded + linked into chapters/TOC
├── cover.png           # optional front cover (or frontcover.*)
├── backcover.png       # optional back cover (or rearcover.*)
├── diagram.png         # any other image - bundled in, unreferenced
├── 01-introduction.md
├── 02-a-second-chapter.md
└── 03-final-thoughts.md
```

Markdown files are processed in filename order, so a numeric prefix (`01-`, `02-`, ...) controls chapter order. Each chapter's title comes from its own leading `# Heading` if present, otherwise from the filename.

## content.yaml

A flat `key: value` file — not full YAML (no nesting, no lists, no multi-line block scalars). Every field is optional.

```yaml
title: Your Book Title
author: You, the Author
language: en
identifier: urn:uuid:12345678-1234-1234-1234-123456789abc
description: A short description of your book in a single line, no line breaks.
publisher: Name of the Publisher
publisher-info: Published by <a href="https://example.com">Some Publisher</a> Books
date: 2026-08-01
```

Surrounding quotes around a value are optional. A UTF-8 BOM (added by some Windows editors) is tolerated.

- `description` also appears, justified, on the title page.
- `date` is written into the book's metadata (`dc:date`); it must be `YYYY`, `YYYY-MM` or `YYYY-MM-DD`, otherwise a warning is printed and it is ignored.
- `publisher-info` is inserted as raw HTML (unescaped) on the title page and takes priority over the plain-text `publisher` field if both are set.

## License

This project is licensed under the [**MIT License**](./LICENSE).