package readability

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
)

func textDensity(s *goquery.Selection, tags []string) float64 {
	textLength := utf8.RuneCountInString(innerText(s))
	if textLength == 0 {
		return 0
	}
	childLength := 0
	selector := strings.Join(tags, ", ")
	s.Find(selector).Each(func(_ int, child *goquery.Selection) {
		childLength += utf8.RuneCountInString(innerText(child))
	})
	return float64(childLength) / float64(textLength)
}

func headerDuplicatesTitle(header *goquery.Selection, title string) bool {
	headerText := normalizeSpace(header.Text())
	title = normalizeSpace(title)
	if headerText == "" || title == "" {
		return false
	}
	if strings.HasSuffix(headerText, ":") && strings.Contains(title, strings.TrimSuffix(headerText, ":")) {
		return true
	}
	for _, sep := range []string{" - ", " | ", " — ", " – "} {
		if strings.HasPrefix(title, headerText+sep) {
			return true
		}
	}
	for _, sep := range []string{"_", " - ", " | ", " — ", " – "} {
		if strings.HasPrefix(title, headerText+sep) {
			return true
		}
	}
	if titlePrefix, titleSuffix, ok := strings.Cut(title, ":"); ok {
		headerPrefix, headerSuffix, headerHasColon := strings.Cut(headerText, ":")
		if headerHasColon && strings.TrimSpace(headerSuffix) != "" && textSimilarity(titlePrefix, headerPrefix) > 0.75 &&
			textSimilarity(titleSuffix, headerSuffix) <= 0.75 {
			return false
		}
	}
	if shortTitleSubsetHeader(header, title) {
		return true
	}
	return headerText == title || textSimilarity(headerText, title) > 0.75
}

func shortTitleSubsetHeader(header *goquery.Selection, title string) bool {
	headerText := normalizeSpace(header.Text())
	return utf8.RuneCountInString(headerText) <= 45 && textContainsAllTokens(title, headerText)
}

func textContainsAllTokens(text, subtext string) bool {
	textTokens := tokenSet(text)
	for _, token := range strings.Fields(tokenizeText(subtext)) {
		if !textTokens[token] {
			return false
		}
	}
	return len(textTokens) > 0
}

func isSkipLinkNode(s *goquery.Selection) bool {
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
	if strings.Contains(classID, "skip-link") || strings.Contains(classID, "skiplink") {
		return true
	}
	text := strings.ToLower(normalizeSpace(s.Text()))
	return utf8.RuneCountInString(text) < 100 && (strings.Contains(text, "skip navigation") || strings.Contains(text, "jump to navigation"))
}

func textSimilarity(textA, textB string) float64 {
	tokensA := tokenSet(textA)
	tokensB := strings.Fields(tokenizeText(textB))
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0
	}
	unique := make([]string, 0, len(tokensB))
	for _, token := range tokensB {
		if !tokensA[token] {
			unique = append(unique, token)
		}
	}
	allB := strings.Join(tokensB, " ")
	if allB == "" {
		return 0
	}
	return 1 - float64(len(strings.Join(unique, " ")))/float64(len(allB))
}

func tokenSet(text string) map[string]bool {
	tokens := strings.Fields(tokenizeText(text))
	result := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		result[token] = true
	}
	return result
}

func tokenizeText(text string) string {
	text = strings.ToLower(text)
	return regexp.MustCompile(`\W+`).ReplaceAllString(text, " ")
}
