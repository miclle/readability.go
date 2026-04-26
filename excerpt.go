package readability

import (
	"bytes"
	stdhtml "html"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func firstExcerptText(s *goquery.Selection, title string) string {
	var excerpt string
	var shortExcerpt string
	normalizedTitle := normalizeSpace(title)
	s.Find("p, div").EachWithBreak(func(_ int, block *goquery.Selection) bool {
		if block.Find("p, div").Length() > 0 {
			return true
		}
		if hasAncestorNodeTag(block.Get(0), "table") {
			return true
		}
		if isBylineCandidate(block) {
			return true
		}
		text := strings.TrimSpace(block.Text())
		fromBreaks := false
		if brExcerpt := excerptBeforeBreak(block); brExcerpt != "" {
			text = brExcerpt
			fromBreaks = true
		}
		if text == "" {
			return true
		}
		normalizedText := normalizeSpace(text)
		if normalizedText != "" && normalizedTitle != "" &&
			len([]rune(normalizedText)) <= len([]rune(normalizedTitle))+20 &&
			(strings.Contains(normalizedTitle, normalizedText) || strings.Contains(normalizedText, normalizedTitle)) {
			return true
		}
		if isBylineText(text) {
			return true
		}
		linkText := normalizeSpace(block.Find("a").Text())
		if linkText != "" && normalizedText == linkText {
			return true
		}
		if !fromBreaks && len([]rune(normalizedText)) < 25 {
			if shortExcerpt == "" {
				shortExcerpt = text
			}
			return true
		}
		excerpt = text
		return false
	})
	if excerpt == "" {
		excerpt = shortExcerpt
	}
	return excerpt
}

func firstStructuredSourceExcerpt(data []byte, title string) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	doc.Find("script, style, noscript").Remove()
	removeHiddenElements(doc.Selection)

	if breadcrumb := firstBreadcrumbExcerpt(doc, title); breadcrumb != "" {
		return breadcrumb
	}
	if short := firstMathHeavyShortExcerpt(doc); short != "" {
		return short
	}
	if topic := firstNavigationTopicTitle(doc); topic != "" {
		return topic
	}
	if doc.Find("body.mediawiki, #mw-content-text").Length() > 0 {
		if coordinates := firstSelectionText(doc.Find("#coordinates").First()); coordinates != "" {
			return coordinates
		}
		var excerpt string
		doc.Find("#mw-content-text p").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			text := strings.TrimSpace(s.Text())
			normalized := normalizeSpace(text)
			if normalized == "" || isMediaWikiLeadNoise(s, normalized) {
				return true
			}
			excerpt = text
			return false
		})
		return excerpt
	}
	return ""
}

func isMediaWikiLeadNoise(s *goquery.Selection, text string) bool {
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
	if strings.Contains(classID, "hatnote") ||
		strings.Contains(classID, "shortdescription") ||
		strings.Contains(classID, "navigation-not-searchable") {
		return true
	}
	lower := strings.ToLower(text)
	return strings.HasPrefix(lower, "see also:") ||
		strings.HasPrefix(lower, "this article is about") ||
		(strings.HasPrefix(lower, "for ") && strings.Contains(lower, ", see "))
}

func firstMathHeavyShortExcerpt(doc *goquery.Document) string {
	if doc.Find("math, mjx-container").Length() == 0 {
		return ""
	}
	return firstSelectionText(doc.Find("p").FilterFunction(func(_ int, s *goquery.Selection) bool {
		text := normalizeSpace(s.Text())
		return text != "" && len([]rune(text)) < 25
	}).First())
}

func firstNavigationTopicTitle(doc *goquery.Document) string {
	return firstSelectionText(doc.Find(".topic-title").FilterFunction(func(_ int, s *goquery.Selection) bool {
		parent := s.Parent()
		return parent.Find("ul, ol").Length() > 0 && strings.Contains(strings.ToLower(attr(parent, "class")), "contents")
	}).First())
}

func firstBreadcrumbExcerpt(doc *goquery.Document, title string) string {
	titleHead := strings.TrimSpace(strings.Split(normalizeSpace(title), "＜")[0])
	var excerpt string
	doc.Find("p, div").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if s.Find("p, div").Length() > 0 {
			return true
		}
		text := normalizeSpace(s.Text())
		if text == "" || !strings.Contains(text, ">") || len([]rune(text)) > 200 {
			return true
		}
		if titleHead != "" && !strings.Contains(text, titleHead) {
			return true
		}
		excerpt = text
		return false
	})
	return excerpt
}

func firstSourceExcerpt(data []byte, parsedExcerpt string) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return ""
	}

	doc.Find("script, style, noscript").Remove()
	removeHiddenElements(doc.Selection)
	var excerpt string
	roots := doc.Find("article, main, [role=main]")
	if roots.Length() == 0 {
		roots = doc.Find("body")
	}
	roots.Find("p, div, pre").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if s.Find("p, div, pre").Length() > 0 {
			return true
		}
		if hasHiddenAncestor(s) {
			return true
		}
		text := strings.TrimSpace(s.Text())
		if text == "" {
			return true
		}
		textCompact := compactSpace(text)
		parsedCompact := compactSpace(parsedExcerpt)
		if normalizeSpace(text) != normalizeSpace(parsedExcerpt) && !strings.HasPrefix(textCompact, parsedCompact) {
			return true
		}
		if strings.HasPrefix(textCompact, parsedCompact) && normalizeSpace(text) != normalizeSpace(parsedExcerpt) {
			text = strings.TrimSpace(firstRunes(text, len([]rune(parsedExcerpt))))
		}
		excerpt = stdhtml.UnescapeString(text)
		return false
	})
	return excerpt
}

var repeatedBRE = regexp.MustCompile(`(?i)(?:<br\s*/?>\s*){2,}`)

func excerptBeforeBreak(s *goquery.Selection) string {
	if s.Find("br").Length() == 0 {
		return ""
	}
	html, err := s.Html()
	if err != nil {
		return ""
	}
	beforeBreak := repeatedBRE.Split(html, 2)[0]
	fragment, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + beforeBreak + "</div>"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(fragment.Find("div").First().Text())
}
