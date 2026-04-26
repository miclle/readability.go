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

	"github.com/PuerkitoBio/goquery"
)

// Article is the content and metadata extracted from an HTML document.
type Article struct {
	// Title is the article headline as detected from metadata or the document title.
	Title string

	// Content is the cleaned article body wrapped in a `<div id="readability-page-1" class="page">` element.
	Content string

	// TextContent is the plain-text form of Content with whitespace normalized.
	TextContent string

	// Length is the rune count of TextContent.
	Length int

	// Excerpt is a short summary derived from metadata or the leading paragraph.
	Excerpt string

	// Byline is the article author or attribution string when one can be detected.
	Byline string

	// Dir is the article text direction ("ltr" / "rtl") when explicitly set on the content root.
	Dir string

	// SiteName is the publishing site name extracted from metadata (e.g. og:site_name).
	SiteName string

	// Lang is the document language taken from the `<html lang>` attribute.
	Lang string

	// PublishedTime is the article publication timestamp from metadata when available.
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
	classesToPreserve []string
	keepClasses       bool
	nbTopCandidates   int
	disableJSONLD     bool
	allowedVideoRegex *regexp.Regexp
	maxElemsToParse   int
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
	if options != nil && options.CharThreshold > 0 && len([]rune(rawTextContent)) < options.CharThreshold {
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
		Length:        len([]rune(textContent)),
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
