package readability

import (
	"math"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

type conditionOptions struct {
	WeightClasses bool
	Config        parserConfig
}

func cleanConditionally(root *goquery.Selection, tag string, options conditionOptions) {
	var nodes []*xhtml.Node
	root.Find(tag).Each(func(_ int, s *goquery.Selection) {
		if node := s.Get(0); node != nil {
			nodes = append(nodes, node)
		}
	})
	for i := len(nodes) - 1; i >= 0; i-- {
		node := nodes[i]
		if node.Parent == nil || hasAncestorNodeTag(node, "code") || hasAncestorDataTable(node) {
			continue
		}
		s := selectionForNode(node)
		if shouldRemoveConditionally(s, tag, options) {
			removeNode(node)
		}
	}
}

func shouldRemoveConditionally(s *goquery.Selection, tag string, options conditionOptions) bool {
	text := innerText(s)
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
	if attr(s, "id") == "contents" || isExpandedBylineActivity(s) {
		return false
	}
	if attr(s, "id") == "comments" || hasAncestorNodeID(s.Get(0), "comments") {
		return false
	}
	if isContinuationMarker(s) {
		return false
	}
	if tag == "table" && isDataTable(s) {
		return false
	}
	if containsDataTable(s) {
		return false
	}
	if strings.Contains(classID, "thumbcaption") && strings.Contains(strings.ToLower(text), "is one of") && s.Find("sup").Length() > 0 {
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

	weight := 0.0
	if options.WeightClasses {
		weight = classWeight(s)
	}
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
	media := preservableMediaCount(s, options.Config)
	li := s.Find("li").Length() - 100
	input := s.Find("input").Length()
	headingDensity := textDensity(s, []string{"h1", "h2", "h3", "h4", "h5", "h6"})
	embedCount := removableEmbedCount(s, options.Config)
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
	if !isList && weight < 25 && density > 0.2+options.Config.linkDensityModifier {
		remove = true
	}
	if weight >= 25 && density > 0.5+options.Config.linkDensityModifier {
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

func containsDataTable(s *goquery.Selection) bool {
	found := false
	s.Find("table").EachWithBreak(func(_ int, table *goquery.Selection) bool {
		if isDataTable(table) {
			found = true
			return false
		}
		return true
	})
	return found
}

func hasAncestorDataTable(node *xhtml.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if tagNameNode(parent) == "table" && isDataTable(selectionForNode(parent)) {
			return true
		}
	}
	return false
}

func isDataTable(s *goquery.Selection) bool {
	if strings.EqualFold(attr(s, "role"), "presentation") || attr(s, "datatable") == "0" {
		return false
	}
	if attr(s, "summary") != "" {
		return true
	}
	if caption := s.Find("caption").First(); caption.Length() > 0 && caption.Get(0).FirstChild != nil {
		return true
	}
	if s.Find("col, colgroup, tfoot, thead, th").Length() > 0 {
		return true
	}
	if s.Find("table").Length() > 0 {
		return false
	}
	rows, columns := tableSize(s)
	if columns == 1 || rows == 1 {
		return false
	}
	if rows >= 10 || columns > 4 {
		return true
	}
	return rows*columns > 10
}

func tableSize(s *goquery.Selection) (int, int) {
	rows := 0
	columns := 0
	s.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		rowspan := positiveIntAttr(tr, "rowspan")
		if rowspan == 0 {
			rowspan = 1
		}
		rows += rowspan

		columnsInRow := 0
		tr.Find("td").Each(func(_ int, td *goquery.Selection) {
			colspan := positiveIntAttr(td, "colspan")
			if colspan == 0 {
				colspan = 1
			}
			columnsInRow += colspan
		})
		if columnsInRow > columns {
			columns = columnsInRow
		}
	})
	return rows, columns
}

func positiveIntAttr(s *goquery.Selection, name string) int {
	value, err := strconv.Atoi(attr(s, name))
	if err != nil || value < 0 {
		return 0
	}
	return value
}
