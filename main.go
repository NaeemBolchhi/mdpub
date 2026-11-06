// mdpub builds a single EPUB from a directory of .md files.
//
// For a source directory it will:
//   - read book metadata (title, author, language, identifier, description,
//     publisher, publisher-info, date) from a content.yaml file in that
//     directory, if present (run with -config to scaffold one)
//   - look for an optional front cover image named "cover" or "frontcover"
//     and place it as the EPUB's cover (first page)
//   - look for an optional back cover image named "backcover" or "rearcover"
//     ("backcover" wins if both exist) and place it as the very last page.
//     For both covers, if more than one image shares the matched base name
//     with different extensions, a .png is preferred.
//   - copy every other image file (raster or vector) alongside the .md
//     files into the EPUB's images folder, unreferenced, for the user to
//     link to manually later
//   - build a simple, top-aligned title page (title, author, description,
//     publisher) right after the cover and before the table of contents
//   - convert every .md file to XHTML with a CommonMark+GFM parser
//     (goldmark), stripping any leading YAML frontmatter (e.g. Obsidian)
//     first, and guaranteeing each chapter has an <h1> title
//   - normalise raw HTML in the chapters to well-formed XHTML and rewrite
//     links between .md files to the matching chapter pages
//   - generate a visible "Table of Contents" page listing every other page
//     in the book (cover, title page, chapters, back cover) - the same set
//     of entries that end up in the EPUB3 nav.xhtml / EPUB2 toc.ncx that
//     reading apps use for their own TOC menus
//   - embed a styles.css found alongside the .md files, if present, and
//     link it into the chapters and the table of contents page
//   - write the optional content.yaml "date" into the package metadata
//   - write out exactly one file: the .epub. No loose .html/.css files are
//     left behind.
//
// Usage:
//
//	mdpub -dir ./book
//	mdpub -dir ./book -out mybook.epub
//	mdpub -config -dir ./book   # scaffold a starter content.yaml and exit
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var dirValue string
	const dirUsage = "directory (or a file path inside it) containing the .md files to convert; aliases: -dir -in -indir -i"
	flag.StringVar(&dirValue, "dir", "", dirUsage)
	flag.StringVar(&dirValue, "in", "", dirUsage)
	flag.StringVar(&dirValue, "indir", "", dirUsage)
	flag.StringVar(&dirValue, "i", "", dirUsage)

	var outValue string
	const outUsage = "where to write the .epub: a directory (created if needed, filename derived from the book title), or a specific file path; default: <title>.epub in the current directory; aliases: -out -outdir -o"
	flag.StringVar(&outValue, "out", "", outUsage)
	flag.StringVar(&outValue, "outdir", "", outUsage)
	flag.StringVar(&outValue, "o", "", outUsage)

	configFlag := flag.Bool("config", false, "write a template content.yaml and exit; can only be combined with the -dir/-in/... and -out/-outdir/... aliases")
	flag.Parse()

	if *configFlag {
		combinedWithOther := false
		flag.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "config", "dir", "in", "indir", "i", "out", "outdir", "o":
			default:
				combinedWithOther = true
			}
		})
		if combinedWithOther {
			fmt.Fprintln(os.Stderr, "error: -config can only be combined with the -dir/-in/... and -out/-outdir/... aliases")
			os.Exit(1)
		}

		// Use whichever location is available: input takes priority over
		// output if both are given; if neither is given, fall back to the
		// current directory.
		target := dirValue
		if target == "" && flag.NArg() > 0 {
			target = flag.Arg(0)
		}
		if target == "" {
			target = outValue
		}
		if target == "" {
			target = "."
		}

		resolved, err := resolveOutputDir(target)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if err := scaffoldContentYAML(resolved); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	dir := dirValue
	if dir == "" && flag.NArg() > 0 {
		dir = flag.Arg(0)
	}
	if dir == "" {
		dir = "."
	}

	if err := run(dir, outValue); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
