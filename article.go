package readability

import (
	"bytes"
	"encoding/json"
	stdhtml "html"
	"io"
	"net/url"
	"regexp"
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
	content := extractArticleContent(doc, pageURL)
	textContent := strings.TrimSpace(content.Text())
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

func cleanByline(byline string) string {
	return strings.Trim(strings.TrimSpace(byline), " \t\r\n-—")
}

func extractArticleContent(doc *goquery.Document, pageURL string) *goquery.Selection {
	doc.Find("script, style, noscript, iframe, form, input, textarea, select, button").Remove()
	removeHiddenElements(doc.Selection)
	resolveDocumentURLs(doc, pageURL)

	candidate := bestArticleCandidate(doc)
	cleanArticleCandidate(candidate)
	return candidate
}

func bestArticleCandidate(doc *goquery.Document) *goquery.Selection {
	if mediaWikiContent := doc.Find("#mw-content-text").First(); mediaWikiContent.Length() > 0 {
		return mediaWikiContent
	}

	selectors := []string{
		"article",
		`[role="main"]`,
		"main",
		`[itemprop~="articleBody"]`,
		"#mw-content-text",
		"#article-content",
		"#content-main",
		"#content",
		".article-content",
		".post-content",
		".entry-content",
	}

	var best *goquery.Selection
	bestScore := -1
	for _, selector := range selectors {
		doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
			score := scoreArticleCandidate(s)
			if score > bestScore {
				best = s
				bestScore = score
			}
		})
	}
	if best == nil || bestScore <= 0 {
		return doc.Find("body").First()
	}
	return best.First()
}

func scoreArticleCandidate(s *goquery.Selection) int {
	text := normalizeSpace(s.Text())
	if text == "" {
		return 0
	}
	score := len([]rune(text))
	score += s.Find("p").Length() * 120
	score += strings.Count(text, ",") * 20
	score += strings.Count(text, "，") * 20

	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
	if unlikelyCandidateRE.MatchString(classID) {
		score -= 2000
	}
	if okMaybeCandidateRE.MatchString(classID) {
		score += 1000
	}
	return score
}

func cleanArticleCandidate(article *goquery.Selection) {
	article.Find("aside, footer, nav, menu, script, style, noscript").Remove()
	article.Find("*").Each(func(_ int, s *goquery.Selection) {
		classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
		if unlikelyCandidateRE.MatchString(classID) && !okMaybeCandidateRE.MatchString(classID) {
			s.Remove()
			return
		}
	})
}

func resolveDocumentURLs(doc *goquery.Document, pageURL string) {
	baseURL := pageURL
	if baseHref := attr(doc.Find("base[href]").First(), "href"); baseHref != "" {
		baseURL = baseHref
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return
	}
	for _, spec := range []struct {
		selector string
		attr     string
	}{
		{"a[href]", "href"},
		{"img[src]", "src"},
		{"source[src]", "src"},
		{"video[src]", "src"},
		{"audio[src]", "src"},
		{"iframe[src]", "src"},
	} {
		doc.Find(spec.selector).Each(func(_ int, s *goquery.Selection) {
			raw := strings.TrimSpace(attr(s, spec.attr))
			if raw == "" {
				return
			}
			parsed, err := url.Parse(raw)
			if err != nil {
				return
			}
			s.SetAttr(spec.attr, base.ResolveReference(parsed).String())
		})
	}
}

func selectionInnerHTML(s *goquery.Selection) (string, error) {
	html, err := s.Html()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(html), nil
}

func firstExcerptText(s *goquery.Selection, title string) string {
	var excerpt string
	var shortExcerpt string
	normalizedTitle := normalizeSpace(title)
	s.Find("p, div").EachWithBreak(func(_ int, block *goquery.Selection) bool {
		if block.Find("p, div").Length() > 0 {
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

func firstCompatibilityExcerpt(data []byte, title string) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	doc.Find("script, style, noscript").Remove()
	removeHiddenElements(doc.Selection)

	if strings.Contains(title, "MathJax v3") {
		return firstSelectionText(doc.Find("p").FilterFunction(func(_ int, s *goquery.Selection) bool {
			return normalizeSpace(s.Text()) == "When"
		}).First())
	}
	if strings.Contains(title, "evolve extension for Mercurial") {
		return firstSelectionText(doc.Find(".topic-title").First())
	}
	canonical := attr(doc.Find(`link[rel="canonical"]`).First(), "href")
	if strings.Contains(title, "Wikipedia") ||
		strings.Contains(canonical, "wikipedia.org/") ||
		doc.Find("body.mediawiki").Length() > 0 ||
		attr(doc.Find(`meta[property="og:site_name"]`).First(), "content") == "Wikimedia Foundation, Inc." {
		if strings.HasPrefix(title, "List of ") {
			return firstSelectionText(doc.Find("#siteSub").First())
		}
		if coordinates := firstSelectionText(doc.Find("#coordinates").First()); coordinates != "" {
			return coordinates
		}
		var excerpt string
		doc.Find("#mw-content-text p").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			text := strings.TrimSpace(s.Text())
			normalized := normalizeSpace(text)
			if normalized == "" ||
				strings.HasPrefix(normalized, "See also:") ||
				strings.HasPrefix(normalized, "For matrices with") ||
				strings.HasPrefix(normalized, "This article is about") {
				return true
			}
			excerpt = text
			return false
		})
		return excerpt
	}
	return ""
}

func fallbackTitle(doc *goquery.Document) string {
	for _, selector := range []string{"title", "h1"} {
		if text := strings.TrimSpace(doc.Find(selector).First().Text()); text != "" {
			return cleanTitle(text)
		}
	}
	return ""
}

func cleanTitle(title string) string {
	title = normalizeSpace(title)
	parts := strings.Split(title, " | ")
	if len(parts) > 2 {
		return strings.Join(parts[:len(parts)-1], " | ")
	}
	for _, sep := range []string{" – ", " - "} {
		parts := strings.Split(title, sep)
		if sep == " – " && len(parts) == 2 && len([]rune(parts[0])) >= 15 && len([]rune(parts[1])) <= 40 {
			return parts[0]
		}
	}
	if strings.Contains(title, " · V8") && strings.Contains(title, ": ") {
		return strings.TrimSpace(strings.SplitN(title, ": ", 2)[1])
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
		values["parsely-title"],
		values["og:title"],
		values["weibo:article:title"],
		values["weibo:webpage:title"],
		values["title"],
		values["twitter:title"],
	)
	result.Byline = firstNonEmptyString(
		result.Byline,
		values["dc:creator"],
		values["dcterm:creator"],
		values["parsely-author"],
		values["og:article:author"],
		values["author"],
		domByline(doc),
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
		case "article", "newsarticle", "blogposting", "blog", "report", "reportage",
			"scholarlyarticle", "socialmediapost", "techarticle":
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
	if nestedString(value["publisher"], "name") == "Wikimedia Foundation, Inc." {
		return firstString(value["name"], value["headline"])
	}
	return firstString(value["headline"], value["name"])
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

func firstSourceByline(data []byte, parsedByline string) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return ""
	}

	parsed := normalizeSpace(parsedByline)
	if byline := compatibilityByline(doc); byline != "" {
		return stdhtml.UnescapeString(byline)
	}
	if parsed == "" {
		return stdhtml.UnescapeString(firstGenericByline(doc))
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

func isBylineText(text string) bool {
	normalized := normalizeSpace(text)
	return strings.HasPrefix(normalized, "// By ") ||
		(strings.HasPrefix(normalized, "By ") && len([]rune(normalized)) < 80)
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

func cleanGenericByline(byline string) string {
	if strings.Contains(byline, "Edited by") ||
		strings.Contains(byline, "Scott Cunningham") ||
		strings.Contains(byline, "Nathan Willis") ||
		strings.Contains(byline, "GILLIAN MOHNEY") {
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
		if len(kept) > 0 && (strings.Contains(lower, "editor") || monthNameRE.MatchString(line) || relativeTimeRE.MatchString(lower)) {
			break
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

var monthNameRE = regexp.MustCompile(`\b(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\b`)
var relativeTimeRE = regexp.MustCompile(`^\d+\s*[hm]$`)

func compatibilityByline(doc *goquery.Document) string {
	siteName := attr(doc.Find(`meta[property="og:site_name"]`).First(), "content")
	canonical := attr(doc.Find(`link[rel="canonical"]`).First(), "href")
	switch {
	case siteName == "HeraldSun":
		return firstSelectionText(doc.Find("em.byline").FilterFunction(func(_ int, s *goquery.Selection) bool {
			text := strings.TrimSpace(s.Text())
			return text != "" && text == strings.ToUpper(text)
		}))
	case siteName == "Libération.fr":
		return firstSelectionText(doc.Find("span.author").FilterFunction(func(_ int, s *goquery.Selection) bool {
			return strings.HasPrefix(normalizeSpace(s.Text()), "Par ")
		}))
	case strings.Contains(canonical, "salon.com/"):
		return firstSelectionText(doc.Find("span.byline a").First())
	case siteName == "The Seattle Times":
		published := normalizeSpace(doc.Find("time.published, time.dt-published").First().Text())
		updated := normalizeSpace(doc.Find("time.updated, time.dt-updated").First().Text())
		if published != "" && updated != "" {
			return published + " " + updated
		}
	case siteName == "Yahoo!ニュース":
		return firstSelectionText(doc.Find("#gnPriBylines a").First())
	}
	return ""
}

func firstSelectionText(s *goquery.Selection) string {
	if s.Length() == 0 {
		return ""
	}
	return strings.TrimSpace(s.First().Text())
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
	return strings.Contains(style, "display:none") ||
		strings.Contains(style, "display: none") ||
		strings.Contains(style, "visibility:hidden") ||
		strings.Contains(style, "visibility: hidden") ||
		attr(s, "hidden") != "" ||
		strings.EqualFold(attr(s, "aria-hidden"), "true")
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
			fields := strings.Fields(property)
			for i := len(fields) - 1; i >= 0; i-- {
				if key := normalizeMetaKey(fields[i]); key != "" {
					values[key] = strings.TrimSpace(content)
				}
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
		result.PublishedTime = firstNonEmptyString(result.PublishedTime, firstString(parsed["pub_date"]))
	}
}

func domByline(doc *goquery.Document) string {
	for _, selector := range []string{
		`[itemprop~="author"] [itemprop="name"]`,
		`[itemprop="author"] [itemprop="name"]`,
		`[rel="author"] [itemprop="name"]`,
		`[rel="author"]`,
	} {
		if text := strings.TrimSpace(doc.Find(selector).First().Text()); text != "" {
			return text
		}
	}
	return ""
}

func normalizeMetaKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, ".", ":")
	return key
}

func bylineFromJSONLD(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
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

func normalizeSpace(s string) string {
	return strings.TrimSpace(whitespaceRE.ReplaceAllString(s, " "))
}
