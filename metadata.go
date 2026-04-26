package readability

import (
	"bytes"
	"encoding/json"
	stdhtml "html"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
)

func fallbackTitle(doc *goquery.Document) string {
	if title := strings.TrimSpace(doc.Find("title").First().Text()); title != "" {
		return articleTitleFromDocument(doc, title)
	}
	if h1 := strings.TrimSpace(doc.Find("h1").First().Text()); h1 != "" {
		return normalizeSpace(h1)
	}
	return ""
}

func articleTitleFromDocument(doc *goquery.Document, title string) string {
	original := normalizeSpace(title)
	current := original
	hadHierarchicalSeparators := false

	matches := titleSeparatorRE.FindAllStringIndex(current, -1)
	if len(matches) > 0 {
		hadHierarchicalSeparators = hierarchicalTitleSeparatorRE.MatchString(current)
		last := matches[len(matches)-1]
		current = original[:last[0]]
		if titleWordCount(current) < 3 {
			first := matches[0]
			current = original[first[1]:]
		}
	} else if strings.Contains(current, ": ") {
		trimmed := strings.TrimSpace(current)
		matchesHeading := false
		doc.Find("h1, h2").EachWithBreak(func(_ int, heading *goquery.Selection) bool {
			if strings.TrimSpace(heading.Text()) == trimmed {
				matchesHeading = true
				return false
			}
			return true
		})
		if !matchesHeading {
			current = original[strings.LastIndex(original, ":")+1:]
			if titleWordCount(current) < 3 {
				current = original[strings.Index(original, ":")+1:]
			} else if titleWordCount(original[:strings.Index(original, ":")]) > 5 {
				current = original
			}
		}
	} else if len([]rune(current)) > 150 || len([]rune(current)) < 15 {
		h1s := doc.Find("h1")
		if h1s.Length() == 1 {
			if h1 := strings.TrimSpace(h1s.First().Text()); h1 != "" {
				current = h1
			}
		}
	}

	current = normalizeSpace(current)
	if titleWordCount(current) <= 4 &&
		(!hadHierarchicalSeparators || titleWordCount(current) != titleWordCount(titleSeparatorRE.ReplaceAllString(original, ""))-1) {
		current = original
	}
	return current
}

func titleWordCount(value string) int {
	return len(strings.Fields(strings.TrimSpace(value)))
}

func cleanTitle(title string) string {
	title = normalizeSpace(title)
	parts := strings.Split(title, " | ")
	if len(parts) > 2 {
		return strings.Join(parts[:len(parts)-1], " | ")
	}
	for _, sep := range []string{" · ", " – ", " - "} {
		parts := strings.Split(title, sep)
		if len(parts) == 2 && len([]rune(parts[0])) >= 15 && len([]rune(parts[1])) <= 40 {
			title = parts[0]
			break
		}
	}
	if before, after, ok := strings.Cut(title, ": "); ok &&
		len([]rune(before)) <= 40 && len([]rune(after)) >= 8 {
		return after
	}
	return title
}

func tagName(s *goquery.Selection) string {
	if len(s.Nodes) == 0 {
		return ""
	}
	return s.Nodes[0].Data
}

type metadata struct {
	Title         string
	Byline        string
	Excerpt       string
	SiteName      string
	PublishedTime string
}

func extractMetadata(data []byte) metadata {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return metadata{}
	}
	result := extractJSONLDMetadata(doc)
	values := collectMetaValues(doc)

	result.Title = firstNonEmptyString(
		result.Title,
		values["dc:title"],
		values["dcterm:title"],
		values["og:title"],
		values["weibo:article:title"],
		values["weibo:webpage:title"],
		values["title"],
		values["twitter:title"],
		values["parsely-title"],
	)
	result.Byline = firstNonEmptyString(
		result.Byline,
		values["dc:creator"],
		values["dcterm:creator"],
		values["author"],
		values["parsely-author"],
		articleAuthor(values["article:author"]),
	)
	result.Excerpt = firstNonEmptyString(
		result.Excerpt,
		values["dc:description"],
		values["dcterm:description"],
		values["og:description"],
		values["weibo:article:description"],
		values["weibo:webpage:description"],
		values["description"],
		values["twitter:description"],
	)
	result.SiteName = firstNonEmptyString(result.SiteName, values["og:site_name"])
	result.PublishedTime = firstNonEmptyString(
		result.PublishedTime,
		values["article:published_time"],
		values["parsely-pub-date"],
		values["dcterms.available"],
		values["dcterms.created"],
		values["dcterms.issued"],
		values["weibo:article:create_at"],
	)

	applyParselyMetadata(values, &result)

	result.Title = stdhtml.UnescapeString(result.Title)
	result.Byline = stdhtml.UnescapeString(result.Byline)
	result.Excerpt = unescapeMetadataString(result.Excerpt)
	result.SiteName = stdhtml.UnescapeString(result.SiteName)
	result.PublishedTime = stdhtml.UnescapeString(result.PublishedTime)
	return result
}

func articleAuthor(value string) string {
	if isURL(value) {
		return ""
	}
	return value
}

func isURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func extractJSONLDMetadata(doc *goquery.Document) metadata {
	var result metadata
	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		raw := strings.TrimSpace(s.Text())
		raw = strings.TrimPrefix(raw, "<![CDATA[")
		raw = strings.TrimSuffix(raw, "]]>")
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return true
		}

		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return true
		}
		result = metadataFromJSONLD(value)
		return result == (metadata{})
	})
	return result
}

func metadataFromJSONLD(value any) metadata {
	switch typed := value.(type) {
	case []any:
		var fallback metadata
		for _, item := range typed {
			result := metadataFromJSONLD(item)
			if result == (metadata{}) {
				continue
			}
			if mapItem, ok := item.(map[string]any); ok && isArticleJSONLD(mapItem) {
				return result
			}
			if fallback == (metadata{}) {
				fallback = result
			}
		}
		return fallback
	case map[string]any:
		if graph, ok := typed["@graph"]; ok {
			if result := metadataFromJSONLD(graph); result != (metadata{}) {
				return result
			}
		}
		if !isArticleJSONLD(typed) {
			return metadata{}
		}
		return metadata{
			Title:         titleFromJSONLD(typed),
			Byline:        bylineFromJSONLD(typed["author"]),
			Excerpt:       unescapeMetadataString(firstString(typed["description"])),
			SiteName:      nestedString(typed["publisher"], "name"),
			PublishedTime: firstString(typed["datePublished"]),
		}
	}
	return metadata{}
}

func isArticleJSONLD(value map[string]any) bool {
	for _, typ := range jsonLDTypes(value["@type"]) {
		switch strings.ToLower(typ) {
		case "article", "advertisercontentarticle", "newsarticle", "analysisnewsarticle",
			"askpublicnewsarticle", "backgroundnewsarticle", "opinionnewsarticle",
			"reportagenewsarticle", "reviewnewsarticle", "report", "satiricalarticle",
			"scholarlyarticle", "medicalscholarlyarticle", "socialmediaposting",
			"blogposting", "liveblogposting", "discussionforumposting", "techarticle",
			"apireference", "blog", "reportage":
			return true
		case "organization", "website", "webpage", "breadcrumblist", "listitem",
			"person", "videoobject", "imageobject":
			return false
		}
	}
	return firstString(value["headline"]) != "" &&
		(firstString(value["datePublished"]) != "" || bylineFromJSONLD(value["author"]) != "")
}

func jsonLDTypes(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		types := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := firstString(item); text != "" {
				types = append(types, text)
			}
		}
		return types
	default:
		return nil
	}
}

func titleFromJSONLD(value map[string]any) string {
	headline := firstString(value["headline"])
	name := firstString(value["name"])
	if name != "" && headline != "" && genericJSONLDHeadline(headline) {
		return name
	}
	return firstNonEmptyString(headline, name)
}

func genericJSONLDHeadline(headline string) bool {
	headline = strings.TrimSpace(headline)
	if headline == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(headline)
	return unicode.IsLower(first) || strings.Contains(strings.ToLower(headline), " article")
}

func removeHiddenElements(root *goquery.Selection) {
	root.Find("*").Each(func(_ int, s *goquery.Selection) {
		if isHidden(s) {
			s.Remove()
		}
	})
}

func hasHiddenAncestor(s *goquery.Selection) bool {
	for current := s; current.Length() > 0; current = current.Parent() {
		if isHidden(current) {
			return true
		}
	}
	return false
}

func firstRunes(value string, n int) string {
	runes := []rune(value)
	if n > len(runes) {
		n = len(runes)
	}
	return string(runes[:n])
}

func compactSpace(value string) string {
	return strings.ReplaceAll(normalizeSpace(value), " ", "")
}

func unescapeMetadataString(value string) string {
	const placeholder = "\x00READABILITY_NBSP\x00"
	value = strings.ReplaceAll(value, "&nbsp;", placeholder)
	value = stdhtml.UnescapeString(value)
	return strings.ReplaceAll(value, placeholder, "&nbsp;")
}

func isHidden(s *goquery.Selection) bool {
	style := strings.ToLower(attr(s, "style"))
	if strings.EqualFold(attr(s, "aria-hidden"), "true") && hasFallbackImageClass(s) {
		return false
	}
	_, hidden := s.Attr("hidden")
	return strings.Contains(style, "display:none") ||
		strings.Contains(style, "display: none") ||
		strings.Contains(style, "visibility:hidden") ||
		strings.Contains(style, "visibility: hidden") ||
		hidden ||
		strings.EqualFold(attr(s, "aria-hidden"), "true")
}

func hasFallbackImageClass(s *goquery.Selection) bool {
	className := strings.ToLower(attr(s, "class"))
	return strings.Contains(className, "fallback-image")
}

func collectMetaValues(doc *goquery.Document) map[string]string {
	values := map[string]string{}
	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		content := attr(s, "content")
		if content == "" {
			return
		}

		property := attr(s, "property")
		if property != "" {
			matches := metaPropertyRE.FindAllString(property, -1)
			if len(matches) > 0 {
				for _, match := range matches {
					key := strings.ToLower(strings.ReplaceAll(match, " ", ""))
					values[key] = strings.TrimSpace(content)
				}
				return
			}
			return
		}

		name := attr(s, "name")
		if key := normalizeMetaKey(name); key != "" {
			values[key] = strings.TrimSpace(content)
		}
	})
	return values
}

func applyParselyMetadata(values map[string]string, result *metadata) {
	for _, key := range []string{"parsely-page", "parsely-metadata"} {
		raw := values[key]
		if raw == "" {
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(stdhtml.UnescapeString(raw)), &parsed); err != nil {
			continue
		}
		result.Title = firstNonEmptyString(result.Title, firstString(parsed["title"]))
		result.Byline = firstNonEmptyString(result.Byline, firstString(parsed["author"]))
		result.Excerpt = firstNonEmptyString(result.Excerpt, firstString(parsed["lower_deck"]))
	}
}

func normalizeMetaKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, ".", ":")
	if strings.HasPrefix(key, "article:") {
		return ""
	}
	return key
}

func bylineFromJSONLD(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		return nestedString(typed, "name")
	case []any:
		var names []string
		for _, item := range typed {
			if name := bylineFromJSONLD(item); name != "" {
				names = append(names, name)
			}
		}
		return strings.Join(names, ", ")
	default:
		return ""
	}
}

func nestedString(value any, key string) string {
	if typed, ok := value.(map[string]any); ok {
		return firstString(typed[key])
	}
	return ""
}

func firstString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func attr(s *goquery.Selection, name string) string {
	value, ok := s.Attr(name)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

var whitespaceRE = regexp.MustCompile(`\s+`)
var metaPropertyRE = regexp.MustCompile(`(?i)\s*(article|dc|dcterm|og|twitter)\s*:\s*(author|creator|description|published_time|title|site_name)\s*`)
var titleSeparatorRE = regexp.MustCompile(`\s[\|\-–—\\/>»]\s`)
var hierarchicalTitleSeparatorRE = regexp.MustCompile(`\s[\\/>»]\s`)

func normalizeSpace(s string) string {
	return strings.TrimSpace(whitespaceRE.ReplaceAllString(s, " "))
}
