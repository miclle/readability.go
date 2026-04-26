package readability

import (
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// Legacy helpers cover older table-based layouts that need normalization but
// should not shape the generic extraction path.
func legacyTableArticleSelection(doc *goquery.Document) *goquery.Selection {
	main := bestLegacyTableCell(doc)
	if main.Length() == 0 {
		return &goquery.Selection{}
	}
	mainNode := main.Get(0)
	sidebar := previousElementSibling(mainNode)
	if mainNode == nil || sidebar == nil {
		return &goquery.Selection{}
	}
	wrapper := &xhtml.Node{Type: xhtml.ElementNode, Data: "div"}
	content := &xhtml.Node{Type: xhtml.ElementNode, Data: "div"}
	wrapper.AppendChild(content)
	for sibling := sidebar; sibling != nil; {
		next := nextElementSibling(sibling)
		appendNode(content, sibling)
		sibling = next
	}
	return goquery.NewDocumentFromNode(wrapper).Selection
}

func bestLegacyTableCell(doc *goquery.Document) *goquery.Selection {
	var best *goquery.Selection
	bestScore := 0.0
	doc.Find("td, th").Each(func(_ int, s *goquery.Selection) {
		if !isLegacyMainTableCell(s) {
			return
		}
		textLength := utf8.RuneCountInString(innerText(s))
		score := float64(textLength) * (1 - linkDensity(s))
		if score <= bestScore {
			return
		}
		best = s
		bestScore = score
	})
	if best == nil {
		return &goquery.Selection{}
	}
	return best
}

func isLegacyMainTableCell(s *goquery.Selection) bool {
	node := s.Get(0)
	if node == nil || tagNameNode(node.Parent) != "tr" {
		return false
	}
	if hasAncestorNodeTag(node.Parent, "td") {
		return false
	}
	if previousElementSibling(node) == nil || nextElementSibling(node) == nil {
		return false
	}
	if utf8.RuneCountInString(innerText(s)) < 300 {
		return false
	}
	return linkDensity(s) < 0.65
}

func cleanLegacyTableCandidate(article *goquery.Selection, cfg parserConfig) {
	replaceJavascriptLinks(article)
	unwrapSingleCellTables(article)
	unwrapSingleCellNestedTablesAsDiv(article)
	normalizeLegacyFileURLs(article)
	article.Find("div, td[colspan='4']").Each(func(_ int, s *goquery.Selection) {
		wrapPhrasingContentInParagraphs(s.Get(0))
	})
	unwrapLegacySidebarParagraphs(article)
	article.Find("*").Each(func(_ int, s *goquery.Selection) {
		cleanPresentationAttributes(s.Get(0), cfg)
	})
	cleanPresentationAttributes(article.Get(0), cfg)
	article.Find("p").Each(func(_ int, s *goquery.Selection) {
		if normalizeSpace(s.Text()) == "" && s.Find("img, audio").Length() == 0 {
			s.Remove()
		}
	})
}

func unwrapLegacySidebarParagraphs(article *goquery.Selection) {
	content := article.ChildrenFiltered("div").First()
	if content.Length() == 0 {
		content = article.Find("div").First()
	}
	sidebar := content.ChildrenFiltered("td").Last()
	if sidebar.Length() == 0 {
		return
	}
	sidebar.Find("td > p").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil || node.Parent == nil || tagNameNode(node.Parent) != "td" {
			return
		}
		for node.FirstChild != nil {
			child := node.FirstChild
			node.RemoveChild(child)
			node.Parent.InsertBefore(child, node)
		}
		removeNode(node)
	})
	sidebar.Find("span > p").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil || node.Parent == nil || tagNameNode(firstElementChild(node)) != "u" {
			return
		}
		for node.FirstChild != nil {
			child := node.FirstChild
			node.RemoveChild(child)
			node.Parent.InsertBefore(child, node)
		}
		removeNode(node)
	})
	collapseLegacySidebarLinkSpacing(sidebar.Get(0))
}

func collapseLegacySidebarLinkSpacing(node *xhtml.Node) {
	if node == nil {
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.TextNode && tagNameNode(nextElementSiblingLike(child)) == "a" {
			child.Data = strings.TrimRight(child.Data, " \t\r\n")
		}
		collapseLegacySidebarLinkSpacing(child)
	}
}

func nextElementSiblingLike(node *xhtml.Node) *xhtml.Node {
	for sibling := node.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type == xhtml.TextNode && strings.TrimSpace(sibling.Data) == "" {
			continue
		}
		if sibling.Type == xhtml.ElementNode {
			return sibling
		}
		return nil
	}
	return nil
}

func unwrapSingleCellNestedTablesAsDiv(root *goquery.Selection) {
	root.Find("table").Each(func(_ int, s *goquery.Selection) {
		table := s.Get(0)
		if table == nil || table.Parent == nil {
			return
		}
		rows := s.ChildrenFiltered("tbody").ChildrenFiltered("tr")
		if rows.Length() == 0 {
			rows = s.ChildrenFiltered("tr")
		}
		cells := rows.ChildrenFiltered("td, th")
		if cells.Length() != 1 {
			return
		}
		innerTable := cells.First().ChildrenFiltered("table").First()
		if innerTable.Length() == 0 || cells.First().Children().Length() != 1 {
			return
		}
		div := &xhtml.Node{Type: xhtml.ElementNode, Data: "div"}
		appendNode(div, innerTable.Get(0))
		replaceNode(table, div)
	})
}

func normalizeLegacyFileURLs(root *goquery.Selection) {
	root.Find("[src], [href]").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		for i := range node.Attr {
			if node.Attr[i].Key != "src" && node.Attr[i].Key != "href" {
				continue
			}
			node.Attr[i].Val = strings.ReplaceAll(node.Attr[i].Val, "file:///C%7C/", "file:///C:/")
			node.Attr[i].Val = strings.ReplaceAll(node.Attr[i].Val, "file:///C|/", "file:///C:/")
		}
	})
}
