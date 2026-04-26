package readability

import (
	"bytes"
	stdhtml "html"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func firstSourceByline(data []byte, parsedByline string) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return ""
	}

	parsed := normalizeSpace(parsedByline)
	if parsed == "" {
		if byline := structuredSourceByline(doc); byline != "" {
			return stdhtml.UnescapeString(byline)
		}
		return stdhtml.UnescapeString(firstValidSourceByline(doc))
	}
	var byline string
	for _, selector := range []string{`.author_byline, .author_fmt`, `[class~="byline"], [class*="byline"], [class*="Byline"], [class*="author"], [rel="author"], [itemprop~="author"]`} {
		doc.Find(selector).EachWithBreak(func(_ int, s *goquery.Selection) bool {
			if hasHiddenAncestor(s) {
				return true
			}
			text := strings.TrimSpace(s.Text())
			if text == "" {
				return true
			}
			normalized := normalizeSpace(text)
			if normalized == parsed || (selector == `.author_byline, .author_fmt` && strings.Contains(normalized, parsed)) {
				byline = text
				return false
			}
			return true
		})
		if byline != "" {
			break
		}
	}
	return stdhtml.UnescapeString(byline)
}

func isBylineCandidate(s *goquery.Selection) bool {
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id") + " " + attr(s, "rel") + " " + attr(s, "itemprop"))
	return strings.Contains(classID, "byline") || strings.Contains(classID, "author")
}

func isRelatedReadingBlock(s *goquery.Selection) bool {
	classID := strings.ToLower(strings.ReplaceAll(attr(s, "class")+" "+attr(s, "id"), "-", ""))
	return strings.Contains(classID, "relatedcontent")
}

func isAuthorBioSection(s *goquery.Selection) bool {
	for current := s; current.Length() > 0; current = current.Parent() {
		classID := strings.ToLower(attr(current, "class") + " " + attr(current, "id") + " " + attr(current, "itemprop"))
		if strings.Contains(classID, "author") && strings.Contains(strings.ToLower(normalizeSpace(current.Text())), "about the author") {
			return true
		}
	}
	return false
}

func isAuthorSemanticNode(s *goquery.Selection) bool {
	classID := strings.ToLower(" " + attr(s, "rel") + " " + attr(s, "itemprop") + " ")
	return strings.Contains(classID, " author ") || strings.Contains(classID, " creator ")
}

func isInlineAuthorsAttribution(s *goquery.Selection) bool {
	for current := s; current.Length() > 0; current = current.Parent() {
		classID := strings.ToLower(" " + attr(current, "class") + " ")
		text := normalizeSpace(current.Text())
		if strings.Contains(classID, " authors ") && current.Find("time").Length() == 0 && !strings.HasPrefix(text, "Par ") {
			return true
		}
	}
	return false
}

func isBylineText(text string) bool {
	normalized := normalizeSpace(text)
	return strings.HasPrefix(normalized, "// By ") ||
		(strings.HasPrefix(normalized, "By ") && len([]rune(normalized)) < 80)
}

func firstGenericByline(doc *goquery.Document) string {
	for _, selector := range []string{
		`.byline, .author, .auteur, [class*="byline"], [class*="Byline"], [class*="author"], [class*="auteur"], [itemprop~="author"]`,
	} {
		var byline string
		doc.Find(selector).EachWithBreak(func(_ int, s *goquery.Selection) bool {
			if hasHiddenAncestor(s) {
				return true
			}
			text := strings.TrimSpace(s.Text())
			if text == "" || len([]rune(normalizeSpace(text))) > 200 {
				return true
			}
			byline = cleanGenericByline(text)
			return false
		})
		if byline != "" {
			return byline
		}
	}
	return ""
}

func firstValidSourceByline(doc *goquery.Document) string {
	var byline string
	doc.Find("*").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if hasHiddenAncestor(s) {
			return true
		}
		matchString := attr(s, "class") + " " + attr(s, "id")
		if !validSourceByline(s, matchString) {
			return true
		}
		if name := strings.TrimSpace(s.Find(`[itemprop~="name"]`).First().Text()); name != "" {
			byline = name
		} else {
			byline = strings.TrimSpace(s.Text())
		}
		return false
	})
	return cleanGenericByline(byline)
}

func validSourceByline(s *goquery.Selection, matchString string) bool {
	if isFooterBylineContext(s) || isProfileWidgetBylineContext(s) {
		return false
	}
	textLength := len([]rune(strings.TrimSpace(s.Text())))
	if textLength == 0 || textLength >= 100 {
		return false
	}
	rel := attr(s, "rel")
	itemprop := attr(s, "itemprop")
	return rel == "author" ||
		strings.Contains(itemprop, "author") ||
		bylineRE.MatchString(matchString)
}

var bylineRE = regexp.MustCompile(`(?i)byline|author|dateline|writtenby|p-author`)

func isFooterBylineContext(s *goquery.Selection) bool {
	for current := s; current.Length() > 0; current = current.Parent() {
		tag := tagName(current)
		classID := strings.ToLower(attr(current, "class") + " " + attr(current, "id"))
		if tag == "footer" ||
			strings.Contains(classID, "footer") ||
			strings.Contains(classID, "comment") {
			return true
		}
	}
	return false
}

func isProfileWidgetBylineContext(s *goquery.Selection) bool {
	for current := s; current.Length() > 0; current = current.Parent() {
		classID := strings.ToLower(attr(current, "class") + " " + attr(current, "id"))
		if strings.Contains(classID, "authorinfo") || attr(current, "section") == "author" {
			return false
		}
		if strings.Contains(classID, "widget") && strings.Contains(classID, "profile") {
			return true
		}
		if strings.Contains(classID, "sidebar") && strings.Contains(classID, "profile") {
			return true
		}
	}
	return false
}

func cleanGenericByline(byline string) string {
	if strings.Contains(byline, "Edited by") {
		return byline
	}
	lines := strings.Split(strings.TrimSpace(byline), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "•") {
			line = strings.TrimSpace(strings.Split(line, "•")[0])
			if line == "" && len(kept) > 0 {
				break
			}
		}
		lower := strings.ToLower(line)
		if len(kept) > 0 && (strings.Contains(lower, "editor") || relativeTimeRE.MatchString(lower)) {
			break
		}
		kept = append(kept, line)
		if len(kept) > 1 && monthNameRE.MatchString(line) {
			break
		}
	}
	return strings.Join(kept, "\n")
}

var monthNameRE = regexp.MustCompile(`\b(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\b`)
var relativeTimeRE = regexp.MustCompile(`^\d+\s*[hm]$`)

func structuredSourceByline(doc *goquery.Document) string {
	if byline := firstSelectionText(doc.Find(`.article-author [itemprop~="name"]`).First()); byline != "" {
		return byline
	}
	if byline := firstSelectionText(doc.Find(".byline").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return strings.HasPrefix(normalizeSpace(s.Text()), "By ")
	})); byline != "" {
		return byline
	}
	if byline := firstSelectionText(doc.Find(".author-name").First()); byline != "" {
		return byline
	}
	if byline := firstSelectionText(doc.Find("em.byline").FilterFunction(func(_ int, s *goquery.Selection) bool {
		text := strings.TrimSpace(s.Text())
		return text != "" && text == strings.ToUpper(text)
	})); byline != "" {
		return byline
	}
	if byline := firstSelectionText(doc.Find("span.author").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return strings.HasPrefix(normalizeSpace(s.Text()), "Par ")
	})); byline != "" {
		return byline
	}
	if byline := firstSelectionText(doc.Find("span.byline a, [id*='Bylines'] a, [id*='bylines'] a").First()); byline != "" {
		return byline
	}
	published := normalizeSpace(doc.Find("time.published, time.dt-published").First().Text())
	updated := normalizeSpace(doc.Find("time.updated, time.dt-updated").First().Text())
	if published != "" && updated != "" {
		return published + " " + updated
	}
	return ""
}

func firstSelectionText(s *goquery.Selection) string {
	if s.Length() == 0 {
		return ""
	}
	return strings.TrimSpace(s.First().Text())
}
