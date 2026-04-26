package readability

import (
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// cleanArticleCandidate cleans the candidate using the parser default config.
// Retained for tests and external callers; production paths use
// cleanArticleCandidateConfig.
func cleanArticleCandidate(article *goquery.Selection) {
	cleanArticleCandidateConfig(article, defaultParserConfig())
}

// cleanArticleCandidateConfig cleans the candidate using the supplied config.
func cleanArticleCandidateConfig(article *goquery.Selection, cfg parserConfig) {
	cleanArticleCandidateWithOptions(article, articleCleanOptions{
		StripUnlikely:      true,
		WeightClasses:      true,
		CleanConditionally: true,
		Config:             cfg,
	})
}

type articleCleanOptions struct {
	StripUnlikely      bool
	WeightClasses      bool
	CleanConditionally bool
	Config             parserConfig
}

// cleanArticleCandidateWithOptions runs the cleanup pipeline in a fixed
// order that matches mozilla/readability's `_cleanConditionally` flow.
// The order matters:
//  1. Remove forms, scripts, navs (structurally definitely-not-content).
//  2. Apply early compat hooks that re-shape known site templates BEFORE
//     scoring-aware cleanup runs (e.g. attachment images need wrapping
//     so they survive the next steps).
//  3. Drop iframes/embeds/audio that aren't on the allow list.
//  4. Remove "noise" blocks that survived structural removal but are
//     pure UI (share buttons, related-posts wells, ...).
//  5. Normalize markup (replace <br><br>, unwrap single-cell tables,
//     merge missing image attrs, strip data-* attrs, ...).
//  6. Conditional cleanup (the link-density-aware pass) — gated by
//     CleanConditionally because retry rungs may disable it.
//  7. Late compat hooks and unlikely-element removal — order chosen so
//     compat hooks see the conditional-cleanup output, then the
//     class/id-based unlikely filter has the final say.
//  8. Drop empty <p> tags and rebuild the article wrapper.
//
// Each step is intentionally idempotent and stateless: re-running the
// pipeline on its own output should produce the same result.
func cleanArticleCandidateWithOptions(article *goquery.Selection, options articleCleanOptions) {
	removeDisallowedArticleElements(article)
	applyEarlyCompatibilityCleanups(article)
	cleanEmbeddedMedia(article, options.Config)
	removeArticleNoiseBlocks(article)
	normalizeArticleMarkup(article)
	if options.CleanConditionally {
		applyConditionalCleanups(article, conditionOptions{WeightClasses: options.WeightClasses, Config: options.Config})
	}
	applyLateCompatibilityCleanups(article)
	if options.StripUnlikely {
		removeUnlikelyArticleElements(article, options.Config)
	}
	removeEmptyParagraphs(article)
	finalizeArticleStructure(article)
}

func removeDisallowedArticleElements(article *goquery.Selection) {
	article.Find("form, fieldset, aside, footer, menu, script, style, noscript, input, textarea, select, button, link").Remove()
	article.Find("#respond").Remove()
	article.Find("nav").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return attr(s, "id") != "adjacent-posts" &&
			attr(s, "role") != "tablist" && s.Find(`[role="tablist"]`).Length() == 0
	}).Remove()
}

func applyEarlyCompatibilityCleanups(article *goquery.Selection) {
	removeDecorativeLightboxControls(article)
	normalizeFallbackContentContainers(article)
	normalizeEntryHeaders(article)
	wrapAttachmentImageLinks(article)
	restoreContinuationLinks(article)
	replaceJavascriptLinks(article)
}

func cleanEmbeddedMedia(article *goquery.Selection, cfg parserConfig) {
	article.Find("iframe, object, embed").Each(func(_ int, s *goquery.Selection) {
		if !isAllowedEmbeddedMedia(s, cfg) {
			s.Remove()
		}
	})
	article.Find("audio").Each(func(_ int, s *goquery.Selection) {
		if attr(s, "src") == "" {
			s.Remove()
		}
	})
}

func isAllowedEmbeddedMedia(s *goquery.Selection, cfg parserConfig) bool {
	node := s.Get(0)
	if node == nil {
		return false
	}
	for _, attr := range node.Attr {
		if cfg.videoAllowed(attr.Val) {
			return true
		}
	}
	if tagNameNode(node) == "object" {
		if html, err := selectionInnerHTML(s); err == nil && cfg.videoAllowed(html) {
			return true
		}
	}
	return false
}

func removeArticleNoiseBlocks(article *goquery.Selection) {
	article.Find("*").Each(func(_ int, s *goquery.Selection) {
		if isRelatedReadingBlock(s) {
			s.Remove()
			return
		}
		if isFollowUpNavigationBlock(s) {
			s.Remove()
			return
		}
		if isLoadingOrControlPlaceholderBlock(s) {
			s.Remove()
			return
		}
		if isBylineCandidate(s) && utf8.RuneCountInString(normalizeSpace(s.Text())) < 200 &&
			!isInsideCollectionHighlights(s) && !hasAncestorNodeID(s.Get(0), "site-content") {
			if hasAncestorNodeID(s.Get(0), "comments") {
				return
			}
			if tagNameNode(s.Get(0)) == "a" && strings.HasPrefix(attr(s, "id"), "ref-") {
				return
			}
			if normalizeSpace(s.Text()) == "" && s.Find("img").Length() > 0 {
				return
			}
			if isAuthorBioSection(s) {
				return
			}
			if isInlineAuthorsAttribution(s) {
				return
			}
			if isExpandedBylineActivity(s) {
				return
			}
			if isAuthorSemanticNode(s) {
				s.Remove()
				return
			}
			if tagNameNode(s.Get(0)) == "time" {
				return
			}
			if s.Find("time").Length() > 0 {
				s.Find("ol, ul, [itemprop~='author'], [rel='author']").Remove()
				if normalizeSpace(s.Text()) == "" {
					s.Remove()
				}
				return
			}
			s.Remove()
		}
	})
	removePlainBylineParagraphs(article)
	removeLeadingMetadataBlocks(article)
}

func isLoadingOrControlPlaceholderBlock(s *goquery.Selection) bool {
	if s.Find("img, picture, video, audio, embed, object, iframe, svg, math").Length() > 0 {
		return false
	}
	text := normalizeSpace(s.Text())
	return isLoadingPlaceholderText(text)
}

func normalizeArticleMarkup(article *goquery.Selection) {
	article.Find("h1").Each(func(_ int, s *goquery.Selection) {
		if node := s.Get(0); node != nil {
			node.Data = "h2"
		}
	})
	article.Find("br").Each(func(_ int, s *goquery.Selection) {
		br := s.Get(0)
		if br != nil && tagNameNode(nextNodeSkippingWhitespace(br.NextSibling)) == "p" {
			removeNode(br)
			return
		}
		if br != nil && tagNameNode(br.Parent) == "p" && nextNodeSkippingWhitespace(br.NextSibling) == nil {
			next := nextElementSibling(br.Parent)
			if tagNameNode(next) == "ul" || tagNameNode(next) == "ol" {
				removeFollowingWhitespace(br)
				removeNode(br)
			}
		}
	})
	unwrapSingleCellTables(article)
	fixLazyImages(article)
	removeUnusedSVGSymbols(article)
}

func applyConditionalCleanups(article *goquery.Selection, options conditionOptions) {
	cleanConditionally(article, "form", options)
	cleanConditionally(article, "fieldset", options)
	cleanConditionally(article, "table", options)
	cleanConditionally(article, "ul", options)
	cleanConditionally(article, "div", options)
}

func applyLateCompatibilityCleanups(article *goquery.Selection) {
	normalizeVideoPlayerContainers(article)
	convertTextOnlyDivsToParagraphs(article)
	normalizeSingleChildContainers(article)
	unwrapSingleParagraphContainers(article)
	removeEmptyMediaHeadings(article)
	restoreContinuationLinks(article)
}

func removeUnlikelyArticleElements(article *goquery.Selection, cfg parserConfig) {
	article.Find("*").Each(func(_ int, s *goquery.Selection) {
		classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
		if attr(s, "role") == "note" {
			s.Remove()
			return
		}
		if attr(s, "id") == "comments" || attr(s, "id") == "adjacent-posts" || hasAncestorNodeID(s.Get(0), "comments") {
			cleanPresentationAttributes(s.Get(0), cfg)
			return
		}
		text := innerText(s)
		if strings.Contains(classID, "like-post-wrapper") ||
			(utf8.RuneCountInString(text) < 200 && strings.Contains(strings.ToLower(text), "like this:") && strings.Contains(strings.ToLower(text), "loading")) ||
			isLoadingPlaceholderText(text) {
			s.Remove()
			return
		}
		if unlikelyCandidateRE.MatchString(classID) && !okMaybeCandidateRE.MatchString(classID) &&
			tagNameNode(s.Get(0)) != "a" && !(tagNameNode(s.Get(0)) == "header" && hasAncestorNodeTag(s.Get(0), "article")) &&
			!hasAncestorNodeTag(s.Get(0), "table") && !hasAncestorNodeTag(s.Get(0), "code") {
			s.Remove()
			return
		}
		cleanPresentationAttributes(s.Get(0), cfg)
	})
	cleanPresentationAttributes(article.Get(0), cfg)
}

func removeEmptyParagraphs(article *goquery.Selection) {
	article.Find("p").Each(func(_ int, s *goquery.Selection) {
		if normalizeSpace(s.Text()) == "" && s.Find("img, picture, video, audio, embed, object, iframe, svg, math").Length() == 0 {
			s.Remove()
		}
	})
}

func finalizeArticleStructure(article *goquery.Selection) {
	wrapLeadingEmphasisContent(article)
	removeMediaSectionHeadings(article)
	removeCompatibilityHorizontalRules(article)
	removeBoundaryHorizontalRules(article.Get(0))
	simplifyNestedElements(article)
	normalizeCollectionContainers(article)
}

func wrapLeadingEmphasisContent(root *goquery.Selection) {
	root.Find("div").AddBack().Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		first := firstElementChild(node)
		if node == nil || first == nil || tagNameNode(nextElementSibling(first)) != "p" {
			return
		}
		switch tagNameNode(first) {
		case "strong", "em", "b", "i":
			wrapPhrasingContentInParagraphs(node)
		}
	})
}

func cleanPresentationAttributes(node *xhtml.Node, cfg parserConfig) {
	if node == nil || node.Type != xhtml.ElementNode {
		return
	}
	inSVG := tagNameNode(node) == "svg" || hasAncestorNodeTag(node, "svg")
	if hasAncestorNodeTag(node, "svg") {
		node.Data = strings.ToLower(node.Data)
	}
	if inSVG {
		normalizeInlineSVGAttributeCasing(node)
	}
	kept := node.Attr[:0]
	for _, attr := range node.Attr {
		key := strings.ToLower(attr.Key)
		if key == "class" {
			if className := preservedClassList(attr.Val, cfg); className != "" {
				attr.Val = className
				kept = append(kept, attr)
			}
			continue
		}
		if inSVG && key == "style" {
			kept = append(kept, attr)
			continue
		}
		if presentationalAttribute[key] {
			continue
		}
		if (key == "width" || key == "height") && deprecatedSizeAttributeElement[tagNameNode(node)] {
			continue
		}
		kept = append(kept, attr)
	}
	node.Attr = kept
}

func normalizeInlineSVGAttributeCasing(node *xhtml.Node) {
	if tagNameNode(node) != "svg" || nodeAttr(node, "version") != "" {
		return
	}
	for i := range node.Attr {
		if node.Attr[i].Key == "viewBox" {
			node.Attr[i].Key = "viewbox"
		}
	}
}

func preservedClassList(className string, cfg parserConfig) string {
	if cfg.keepClasses {
		return strings.TrimSpace(className)
	}
	allow := make(map[string]struct{}, len(cfg.classesToPreserve)+1)
	allow["page"] = struct{}{}
	for _, class := range cfg.classesToPreserve {
		if class != "" {
			allow[class] = struct{}{}
		}
	}
	var preserved []string
	for _, class := range strings.Fields(className) {
		if _, ok := allow[class]; ok {
			preserved = append(preserved, class)
		}
	}
	return strings.Join(preserved, " ")
}

func removeLeadingMetadataBlocks(root *goquery.Selection) {
	for {
		first := firstElementChild(root.Get(0))
		if first == nil || tagNameNode(first) != "p" {
			return
		}
		s := selectionForNode(first)
		if !isLeadingMetadataBlock(s) {
			return
		}
		removeNode(first)
	}
}

func removePlainBylineParagraphs(root *goquery.Selection) {
	removed := false
	seenBodyText := false
	root.Find("p").Each(func(_ int, s *goquery.Selection) {
		if removed || seenBodyText {
			return
		}
		text := normalizeSpace(s.Text())
		if text == "" {
			return
		}
		if utf8.RuneCountInString(text) >= 120 || !strings.HasPrefix(text, "By ") {
			seenBodyText = true
			return
		}
		if monthNameRE.MatchString(text) || leadingDateRE.MatchString(strings.TrimPrefix(text, "By ")) {
			s.Remove()
			removed = true
			return
		}
		seenBodyText = true
	})
}

func isLeadingMetadataBlock(s *goquery.Selection) bool {
	text := normalizeSpace(s.Text())
	if text == "" || utf8.RuneCountInString(text) > 120 || s.Find("a, img, iframe, video, audio").Length() > 0 {
		return false
	}
	if leadingDateRE.MatchString(text) {
		return true
	}
	lower := strings.ToLower(text)
	return s.Find("time").Length() > 0 &&
		(strings.Contains(lower, "mis à jour") || strings.Contains(lower, " par "))
}

func isFollowUpNavigationBlock(s *goquery.Selection) bool {
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
	return strings.Contains(classID, "whats-next") ||
		strings.Contains(classID, "what-next") ||
		strings.Contains(classID, "read-next")
}

func convertTextOnlyDivsToParagraphs(root *goquery.Selection) {
	root.Find("div").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil || node.Parent == nil || strings.HasPrefix(attr(s, "id"), "readability") {
			return
		}
		if normalizeSpace(s.Text()) == "" || hasChildBlockElement(node) {
			return
		}
		if s.Find("picture, video, audio, iframe, object, embed, svg, math").Length() > 0 {
			return
		}
		node.Data = "p"
		if hasAncestorNodeTag(node, "figcaption") {
			removeNodeAttr(node, "data-reactid")
		}
	})
}

func normalizeSingleChildContainers(root *goquery.Selection) {
	root.Find("div").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil || node.Parent == nil || strings.HasPrefix(attr(s, "id"), "readability") {
			return
		}
		child := firstElementChild(node)
		if child == nil || nextElementSibling(child) != nil || normalizeSpace(s.Text()) == "" {
			return
		}
		switch tagNameNode(child) {
		case "figcaption":
			node.Data = "p"
		default:
			if isHeadingTag(tagNameNode(child)) {
				node.Data = "p"
			}
		}
	})
}

func unwrapSingleParagraphContainers(root *goquery.Selection) {
	root.Find("div").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil || node.Parent == nil || strings.HasPrefix(attr(s, "id"), "readability") {
			return
		}
		if len(node.Attr) > 0 {
			return
		}
		if tagNameNode(node.Parent) == "figure" {
			return
		}
		if !hasSingleElementChild(node, "p") || linkDensity(s) >= 0.25 {
			return
		}
		child := firstElementChild(node)
		if selectionForNode(child).Find("iframe, video, audio").Length() > 0 {
			mergeMissingAttributes(child, node)
		}
		replaceNode(node, child)
	})
}

func removeBoundaryHorizontalRules(root *xhtml.Node) {
	if root == nil {
		return
	}
	for {
		first := firstElementChild(root)
		if tagNameNode(first) != "hr" {
			break
		}
		removeNode(first)
	}
	for {
		last := lastElementChild(root)
		if tagNameNode(last) != "hr" {
			break
		}
		removeNode(last)
	}
}

func replaceJavascriptLinks(root *goquery.Selection) {
	root.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(attr(s, "href"))), "javascript:") {
			return
		}
		link := s.Get(0)
		if link == nil || link.Parent == nil {
			return
		}
		if link.FirstChild != nil && link.FirstChild == link.LastChild && link.FirstChild.Type == xhtml.TextNode {
			replaceNode(link, &xhtml.Node{Type: xhtml.TextNode, Data: link.FirstChild.Data})
			return
		}
		span := &xhtml.Node{Type: xhtml.ElementNode, Data: "span"}
		for link.FirstChild != nil {
			child := link.FirstChild
			link.RemoveChild(child)
			span.AppendChild(child)
		}
		replaceNode(link, span)
	})
}

func removeFollowingWhitespace(node *xhtml.Node) {
	for next := node.NextSibling; next != nil && isWhitespaceNode(next); {
		current := next
		next = next.NextSibling
		removeNode(current)
	}
}
