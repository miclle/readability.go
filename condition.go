package readability

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// conditionOptions carries the knobs that conditional cleanup needs:
// WeightClasses determines whether class/id-based scoring penalties
// apply (retry passes disable it to recover from over-aggressive
// removal), and Config carries through user-facing settings such as
// LinkDensityModifier and AllowedVideoRegex that influence per-element
// decisions inside shouldRemoveConditionally.
type conditionOptions struct {
	WeightClasses bool
	Config        parserConfig
}

// cleanConditionally walks every `tag` element under root in reverse
// document order and removes the ones shouldRemoveConditionally rejects.
// Reverse order matters: the heuristic looks at descendants (image
// counts, link density, embed counts), so processing leaves before
// their ancestors prevents an outer wrapper from being scored against
// children that were already removed. <code> descendants and elements
// inside data tables are skipped because conditional cleanup
// over-aggressively strips legitimate code listings and tabular data.
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

// shouldRemoveConditionally returns true when the element looks like
// chrome rather than article content. Mirrors mozilla/readability's
// `_cleanConditionally` predicate. The decision is the OR of several
// heuristics applied to the element's text, structural counts, and
// class weight; any single positive verdict triggers removal:
//
//   - non-list with > 1 image and a paragraph/image ratio < 0.5: photo
//     gallery rather than prose;
//   - non-list with more <li> than <p> (after a -100 fudge): list
//     masquerading as content;
//   - <input> count > p/3: form;
//   - low heading density + short text + few-or-many images + has
//     links: navigational chrome;
//   - low class weight + link density above 0.2 (+ user modifier):
//     classic boilerplate;
//   - high class weight tolerated up to link density 0.5;
//   - one embed with very little text or multiple embeds: widget block;
//   - no images, no media, and no useful-text-tag density: empty
//     wrapper.
//
// Several pre-checks short-circuit to "keep": continuation markers,
// data tables, tablists, comment threads, image captions, and a few
// site-specific id matches that production fixtures rely on. Negative
// classWeight is treated as a hard remove regardless of other signals.
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
			listLength += utf8.RuneCountInString(innerText(list))
		})
		isList = float64(listLength)/float64(utf8.RuneCountInString(text)) > 0.9
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
	contentLength := utf8.RuneCountInString(text)
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

// containsDataTable reports whether s has any descendant <table> that
// isDataTable accepts. Used as a "keep" short-circuit in
// shouldRemoveConditionally: a wrapper that hosts a real data table is
// presumed to be content even if its other signals look like chrome.
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

// hasAncestorDataTable walks up the parent chain looking for an
// enclosing data table. cleanConditionally uses this to skip cells
// inside legitimate tabular data — the per-element heuristics would
// otherwise strip <td>/<tr> wrappers that happen to score like chrome.
func hasAncestorDataTable(node *xhtml.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if tagNameNode(parent) == "table" && isDataTable(selectionForNode(parent)) {
			return true
		}
	}
	return false
}

// isDataTable classifies a <table> as carrying real tabular data
// (vs. layout/presentation chrome). Mirrors mozilla/readability's
// `_isDataTable` cascade:
//
//   - role="presentation" or datatable="0" is an explicit opt-out;
//   - a non-empty `summary` attribute or a non-empty <caption> is an
//     explicit opt-in;
//   - structural signals (<col>/<colgroup>/<tfoot>/<thead>/<th>) imply
//     data semantics;
//   - a nested <table> implies the outer is layout, not data;
//   - finally, geometry: 1×N or N×1 is layout-ish; ≥10 rows or >4
//     columns or >10 total cells is treated as data.
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

// tableSize returns (rows, columns) for a table, honoring rowspan and
// colspan so merged cells contribute their full footprint. Columns is
// the maximum columns-in-row across all rows rather than a sum, which
// matches how isDataTable's geometry thresholds want to read the table.
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

// positiveIntAttr reads an integer attribute and clamps anything
// non-numeric or negative to 0. tableSize uses it for rowspan/colspan
// where a missing or malformed value should fall back to "1 cell" via
// the caller's `if v == 0 { v = 1 }` guard rather than crashing.
func positiveIntAttr(s *goquery.Selection, name string) int {
	value, err := strconv.Atoi(attr(s, name))
	if err != nil || value < 0 {
		return 0
	}
	return value
}
