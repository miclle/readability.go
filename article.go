// Package readability is a Go port of mozilla/readability that extracts the
// main article content and metadata from an HTML document. The entry points
// are FromReader for full extraction and IsProbablyReaderable for a fast
// pre-check.
package readability

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
)

// Article is the content and metadata extracted from an HTML document.
//
// The struct mirrors the shape returned by mozilla/readability's `parse()`
// so it can be consumed in roughly the same way. All fields are best-effort:
// when no useful value is found, the zero value (empty string / 0) is
// returned rather than an error. Use Length to distinguish "we found an
// article" from "we found nothing readable".
type Article struct {
	// Title is the article headline as detected from metadata
	// (JSON-LD > og:title > twitter:title > <title>) or, if none of
	// those produce a candidate, fallbackTitle's heuristics over the
	// document. Empty when no title can be derived.
	Title string

	// Content is the cleaned article HTML, wrapped in
	// `<div id="readability-page-1" class="page">…</div>`. Suitable for
	// direct rendering (after sanitization for your trust model). Class
	// attributes inside the wrapper are filtered by KeepClasses /
	// ClassesToPreserve; other attributes follow upstream behavior
	// (presentational attrs stripped, deprecated width/height removed
	// from table-related elements).
	Content string

	// TextContent is the plain-text projection of Content with whitespace
	// normalized (runs of whitespace collapse to a single space). Useful
	// for length checks, search indexing, and anything that needs the
	// reading-order text without HTML.
	TextContent string

	// Length is utf8.RuneCountInString(TextContent). Provided so callers
	// can apply CharThreshold-style policies without recounting.
	Length int

	// Excerpt is a short summary drawn from metadata
	// (description / og:description / JSON-LD description) or, if none
	// is present, the first qualifying paragraph of the article body.
	// Empty when neither source produces a candidate.
	Excerpt string

	// Byline is the article author or attribution string. Sourced first
	// from the pristine (pre-cleanup) document so author elements that
	// would otherwise be stripped are preserved, then from JSON-LD /
	// meta tags. Empty when no byline can be detected.
	Byline string

	// Dir is the article text direction ("ltr" or "rtl") when one is
	// explicitly set on the content root or any ancestor; empty otherwise.
	// Useful for rendering UIs that need to mirror layout for RTL content.
	Dir string

	// SiteName is the publisher / site name extracted from metadata,
	// preferring og:site_name. Empty when not present.
	SiteName string

	// Lang is the document language taken from the `<html lang>`
	// attribute on the parsed document. Empty when the attribute is
	// missing.
	Lang string

	// PublishedTime is the article publication timestamp extracted from
	// metadata when available (typically datePublished in JSON-LD or
	// article:published_time in OpenGraph). The string format is
	// preserved verbatim from the source — callers needing a typed
	// time.Time should parse it themselves to avoid lossy normalization.
	PublishedTime string
}

// Options controls parser behavior. Mirrors the public knobs of mozilla/readability.
type Options struct {
	// CharThreshold is the minimum extracted text length required to return an
	// article. A zero value disables the threshold.
	CharThreshold int

	// ClassesToPreserve lists CSS class names that survive cleanup even when
	// KeepClasses is false. The wrapper class "page" is always preserved.
	// A nil slice falls back to ["caption"] to match mozilla/readability.
	ClassesToPreserve []string

	// KeepClasses, when true, preserves all class attributes during cleanup.
	KeepClasses bool

	// NbTopCandidates caps how many top-scoring candidates are tracked while
	// scoring. A zero or negative value falls back to 5.
	NbTopCandidates int

	// DisableJSONLD skips JSON-LD metadata extraction when true.
	DisableJSONLD bool

	// AllowedVideoRegex overrides the built-in allow-list used to recognize
	// embeddable video URLs. A nil value falls back to the built-in regex.
	AllowedVideoRegex *regexp.Regexp

	// MaxElemsToParse aborts parsing when the document contains more than
	// this many elements. Zero or negative disables the limit.
	MaxElemsToParse int

	// LinkDensityModifier shifts the link-density thresholds used by the
	// conditional cleanup pass. The default of 0 matches mozilla/readability;
	// positive values relax cleanup (allow higher link density), negative
	// values tighten it. Mirrors upstream's `linkDensityModifier` option.
	LinkDensityModifier float64
}

// ErrTooManyElements is returned when MaxElemsToParse is exceeded.
var ErrTooManyElements = errors.New("readability: document exceeds MaxElemsToParse")

// ErrBelowCharThreshold is returned when the extracted article's text
// length is shorter than Options.CharThreshold. The returned Article is
// the zero value; callers can use errors.Is to distinguish this case from
// a successful extraction that simply produced no content.
var ErrBelowCharThreshold = errors.New("readability: extracted text below CharThreshold")

// parserConfig is the resolved, internal form of Options.
type parserConfig struct {
	classesToPreserve   []string
	keepClasses         bool
	nbTopCandidates     int
	disableJSONLD       bool
	allowedVideoRegex   *regexp.Regexp
	maxElemsToParse     int
	linkDensityModifier float64
}

// defaultClassesToPreserve mirrors mozilla/readability's default of ["caption"].
// "page" is always preserved separately because it is applied to the wrapper
// div produced by this parser.
var defaultClassesToPreserve = []string{"caption"}

func newParserConfig(options *Options) parserConfig {
	cfg := parserConfig{
		classesToPreserve: defaultClassesToPreserve,
		nbTopCandidates:   5,
	}
	if options == nil {
		return cfg
	}
	if options.ClassesToPreserve != nil {
		cfg.classesToPreserve = options.ClassesToPreserve
	}
	cfg.keepClasses = options.KeepClasses
	if options.NbTopCandidates > 0 {
		cfg.nbTopCandidates = options.NbTopCandidates
	}
	cfg.disableJSONLD = options.DisableJSONLD
	cfg.allowedVideoRegex = options.AllowedVideoRegex
	if options.MaxElemsToParse > 0 {
		cfg.maxElemsToParse = options.MaxElemsToParse
	}
	cfg.linkDensityModifier = options.LinkDensityModifier
	return cfg
}

func (cfg parserConfig) videoAllowed(value string) bool {
	if cfg.allowedVideoRegex != nil {
		return cfg.allowedVideoRegex.MatchString(value)
	}
	return videoURLRE.MatchString(value)
}

func defaultParserConfig() parserConfig {
	return newParserConfig(nil)
}

// FromReader extracts the main article from an HTML stream.
//
// pageURL is used to resolve relative URLs (href, src, srcset) inside
// the extracted content. Pass an empty string to skip URL resolution
// when the input is already absolute or when relative URLs are
// acceptable.
//
// options may be nil, in which case the parser uses upstream-compatible
// defaults: NbTopCandidates=5, ClassesToPreserve=["caption"], no element
// or character thresholds, JSON-LD enabled, and the built-in video
// allow-list. See Options for per-knob behavior.
//
// Errors:
//   - the underlying reader's read error is returned wrapped only if it
//     comes from io.ReadAll;
//   - ErrTooManyElements when the document exceeds Options.MaxElemsToParse;
//   - ErrBelowCharThreshold when the extracted text is shorter than
//     Options.CharThreshold (Article is the zero value in this case);
//   - errors from the underlying HTML parser / serializer when the input
//     is malformed in ways the tolerant html package cannot recover from.
//
// Use errors.Is to distinguish ErrTooManyElements / ErrBelowCharThreshold
// from generic I/O failures.
func FromReader(r io.Reader, pageURL string, options *Options) (Article, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Article{}, err
	}

	cfg := newParserConfig(options)

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return Article{}, err
	}

	if cfg.maxElemsToParse > 0 && doc.Find("*").Length() > cfg.maxElemsToParse {
		return Article{}, ErrTooManyElements
	}

	metadata := extractMetadataConfigDoc(doc, cfg)
	title := fallbackTitle(doc)
	if metadata.Title != "" {
		title = metadata.Title
	}

	// Capture byline candidates from the pristine document before
	// extractArticleContent removes byline-bearing nodes during cleanup.
	pristineSourceByline := firstSourceBylineDoc(doc, metadata.Byline)

	content := extractArticleContent(doc, pageURL, title, cfg)
	rawTextContent := strings.TrimSpace(content.Text())
	if options != nil && options.CharThreshold > 0 && utf8.RuneCountInString(rawTextContent) < options.CharThreshold {
		return Article{}, ErrBelowCharThreshold
	}
	nbspEntity := preferredNBSPSerializedEntity(data)
	textContent := normalizeTextContentEntities(rawTextContent, nbspEntity == "&nbsp;")
	excerpt := firstExcerptText(content, title)
	if metadata.Excerpt != "" {
		excerpt = metadata.Excerpt
	} else if excerpt != "" {
		if sourceExcerpt := firstSourceExcerpt(data, excerpt); sourceExcerpt != "" {
			excerpt = sourceExcerpt
		}
	} else if sourceExcerpt := firstStructuredSourceExcerpt(data, title); sourceExcerpt != "" {
		excerpt = sourceExcerpt
	}

	byline := metadata.Byline
	if pristineSourceByline != "" {
		byline = pristineSourceByline
	}

	htmlContent, err := selectionInnerHTMLWithNBSP(content, nbspEntity)
	if err != nil {
		return Article{}, err
	}

	result := Article{
		Title:         title,
		Content:       `<div id="readability-page-1" class="page">` + htmlContent + `</div>`,
		TextContent:   textContent,
		Length:        utf8.RuneCountInString(textContent),
		Excerpt:       excerpt,
		Byline:        byline,
		Dir:           articleDirection(content),
		SiteName:      "",
		Lang:          attr(doc.Find("html").First(), "lang"),
		PublishedTime: "",
	}

	if metadata.SiteName != "" {
		result.SiteName = metadata.SiteName
	}
	if metadata.PublishedTime != "" {
		result.PublishedTime = metadata.PublishedTime
	}

	return result, nil
}

func preferredNBSPSerializedEntity(data []byte) string {
	numeric := bytes.Count(data, []byte("&#160;"))
	if numeric > 0 {
		return "&#160;"
	}
	return "&nbsp;"
}

func normalizeTextContentEntities(value string, preserveNamedNBSP bool) string {
	if !preserveNamedNBSP {
		return value
	}
	return strings.ReplaceAll(value, "\u00a0", "&nbsp;")
}

func articleDirection(content *goquery.Selection) string {
	if dir := attr(content, "dir"); dir != "" {
		return dir
	}
	return ""
}
