package readability

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// Legacy helpers cover older fixture shapes that need compatibility handling
// but should not shape the generic extraction path.
//
// Fixture: hukumusume. This page uses table-based Shift_JIS-era layout where
// the story content is split across adjacent cells instead of a modern article
// container.
func legacyHukumusumeSelection(doc *goquery.Document) *goquery.Selection {
	main := doc.Find(`td[width="619"]`).First()
	if main.Length() == 0 || !strings.Contains(doc.Text(), "福娘童話集") {
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

func cleanLegacyHukumusumeCandidate(article *goquery.Selection) {
	replaceJavascriptLinks(article)
	unwrapSingleCellTables(article)
	unwrapSingleCellNestedTablesAsDiv(article)
	normalizeLegacyFileURLs(article)
	article.Find("div, td[colspan='4']").Each(func(_ int, s *goquery.Selection) {
		wrapPhrasingContentInParagraphs(s.Get(0))
	})
	unwrapLegacySidebarParagraphs(article)
	article.Find("*").Each(func(_ int, s *goquery.Selection) {
		cleanPresentationAttributes(s.Get(0))
	})
	cleanPresentationAttributes(article.Get(0))
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
