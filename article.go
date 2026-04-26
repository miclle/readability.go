package readability

import (
	"bytes"
	"io"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Article is the content and metadata extracted from an HTML document.
type Article struct {
	Title         string
	Content       string
	TextContent   string
	Length        int
	Excerpt       string
	Byline        string
	Dir           string
	SiteName      string
	Lang          string
	PublishedTime string
}

// Options controls parser behavior. It mirrors Mozilla Readability options as
// the Go implementation grows into full fixture compatibility.
type Options struct {
	// CharThreshold is the minimum extracted text length required to return an
	// article. A zero value disables the threshold.
	CharThreshold int
}

// FromReader extracts the main article from an HTML stream.
func FromReader(r io.Reader, pageURL string, options *Options) (Article, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Article{}, err
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return Article{}, err
	}

	metadata := extractMetadata(data)
	title := fallbackTitle(doc)
	if metadata.Title != "" {
		title = metadata.Title
	}
	content := extractArticleContent(doc, pageURL, title)
	textContent := strings.TrimSpace(content.Text())
	if options != nil && options.CharThreshold > 0 && len([]rune(textContent)) < options.CharThreshold {
		return Article{}, nil
	}
	excerpt := firstExcerptText(content, title)
	if metadata.Excerpt != "" {
		excerpt = metadata.Excerpt
	} else if compatibilityExcerpt := firstCompatibilityExcerpt(data, title); compatibilityExcerpt != "" {
		excerpt = compatibilityExcerpt
	} else if sourceExcerpt := firstSourceExcerpt(data, excerpt); sourceExcerpt != "" {
		excerpt = sourceExcerpt
	}

	byline := ""
	if metadata.Byline != "" {
		byline = metadata.Byline
	}
	if sourceByline := firstSourceByline(data, byline); sourceByline != "" {
		byline = sourceByline
	}

	htmlContent, err := selectionInnerHTML(content)
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
		Dir:           "",
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
