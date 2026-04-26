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

// Options controls parser behavior.
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
	rawTextContent := strings.TrimSpace(content.Text())
	if options != nil && options.CharThreshold > 0 && len([]rune(rawTextContent)) < options.CharThreshold {
		return Article{}, nil
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

	byline := ""
	if metadata.Byline != "" {
		byline = metadata.Byline
	}
	if sourceByline := firstSourceByline(data, byline); sourceByline != "" {
		byline = sourceByline
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
