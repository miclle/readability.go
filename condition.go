package readability

import (
	"math"
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

func cleanConditionally(root *goquery.Selection, tag string) {
	var nodes []*xhtml.Node
	root.Find(tag).Each(func(_ int, s *goquery.Selection) {
		if node := s.Get(0); node != nil {
			nodes = append(nodes, node)
		}
	})
	for i := len(nodes) - 1; i >= 0; i-- {
		node := nodes[i]
		if node.Parent == nil || hasAncestorNodeTag(node, "code") || hasAncestorNodeTag(node, "table") {
			continue
		}
		s := selectionForNode(node)
		if shouldRemoveConditionally(s, tag) {
			removeNode(node)
		}
	}
}

func shouldRemoveConditionally(s *goquery.Selection, tag string) bool {
	text := innerText(s)
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
	if isSmartAssetContainer(s) {
		return false
	}
	if attr(s, "id") == "contents" || isExpandedBylineActivity(s) {
		return false
	}
	if attr(s, "id") == "comments" || hasAncestorNodeID(s.Get(0), "comments") {
		return false
	}
	if isStoryContinuation(s) {
		return false
	}
	if strings.Contains(classID, "thumbcaption") && strings.Contains(strings.ToLower(text), "is one of") && s.Find("sup").Length() > 0 {
		return false
	}
	if isNYTimesCollectionCardSummary(s) {
		return false
	}
	if containsCollectionHighlights(s) {
		return false
	}
	if attr(s, "role") == "tablist" || s.Find(`[role="tablist"]`).Length() > 0 {
		return false
	}
	if s.Find("img").Length() > 0 && (strings.Contains(classID, "thumb") || strings.Contains(classID, "image")) {
		return false
	}
	isList := tag == "ul" || tag == "ol"
	if !isList {
		listLength := 0
		s.Find("ul, ol").Each(func(_ int, list *goquery.Selection) {
			listLength += len([]rune(innerText(list)))
		})
		isList = float64(listLength)/float64(len([]rune(text))) > 0.9
	}

	weight := classWeight(s)
	if weight < 0 {
		return true
	}
	if commaCount(text) >= 10 {
		return false
	}
	if adWordsRE.MatchString(text) || loadingWordsRE.MatchString(text) {
		return true
	}

	p := s.Find("p").Length()
	img := s.Find("img").Length()
	media := preservableMediaCount(s)
	li := s.Find("li").Length() - 100
	input := s.Find("input").Length()
	headingDensity := textDensity(s, []string{"h1", "h2", "h3", "h4", "h5", "h6"})
	embedCount := removableEmbedCount(s)
	contentLength := len([]rune(text))
	density := linkDensity(s)
	textishTags := []string{"span", "li", "td", "address", "blockquote", "dd", "div", "dl", "dt", "figcaption", "h1", "h2", "h3", "h4", "h5", "h6", "p", "pre", "time"}
	usefulTextDensity := textDensity(s, textishTags)
	isFigureChild := hasAncestorNodeTag(s.Get(0), "figure")

	remove := false
	if !isFigureChild && img > 1 && float64(p)/float64(img) < 0.5 {
		remove = true
	}
	if !isList && li > p {
		remove = true
	}
	if input > int(math.Floor(float64(p)/3)) {
		remove = true
	}
	if !isList && !isFigureChild && headingDensity < 0.9 && contentLength < 25 && (img == 0 || img > 2) && density > 0 {
		remove = true
	}
	if !isList && weight < 25 && density > 0.2 {
		remove = true
	}
	if weight >= 25 && density > 0.5 {
		remove = true
	}
	if (embedCount == 1 && contentLength < 75) || embedCount > 1 {
		remove = true
	}
	if img == 0 && media == 0 && usefulTextDensity == 0 {
		remove = true
	}
	if isList && remove {
		children := elementChildren(s.Get(0))
		for _, child := range children {
			if elementChildCount(child) > 1 {
				return remove
			}
		}
		if img == s.Find("li").Length() {
			return false
		}
	}
	return remove
}
