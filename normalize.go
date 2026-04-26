package readability

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

func replaceBreaks(root *goquery.Selection) {
	if root.Length() == 0 {
		return
	}
	var brs []*xhtml.Node
	root.Find("br").Each(func(_ int, brSel *goquery.Selection) {
		if br := brSel.Get(0); br != nil {
			brs = append(brs, br)
		}
	})
	for _, br := range brs {
		if br.Parent == nil {
			continue
		}
		next := br.NextSibling
		replaced := false
		for {
			next = nextNodeSkippingWhitespace(next)
			if tagNameNode(next) != "br" {
				break
			}
			replaced = true
			brSibling := next.NextSibling
			removeNode(next)
			next = brSibling
		}
		if !replaced {
			continue
		}
		p := &xhtml.Node{Type: xhtml.ElementNode, Data: "p"}
		replaceNode(br, p)
		next = p.NextSibling
		for next != nil {
			if tagNameNode(next) == "br" {
				nextElem := nextNodeSkippingWhitespace(next.NextSibling)
				if tagNameNode(nextElem) == "br" {
					break
				}
			}
			if !isPhrasingNode(next) {
				break
			}
			sibling := next.NextSibling
			appendNode(p, next)
			next = sibling
		}
	}
	root.Find("p").Each(func(_ int, pSel *goquery.Selection) {
		if pSel.ChildrenFiltered("p").Length() == 0 {
			return
		}
		if node := pSel.Get(0); node != nil {
			node.Data = "div"
		}
	})
}

func nextNodeSkippingWhitespace(node *xhtml.Node) *xhtml.Node {
	for node != nil && node.Type != xhtml.ElementNode && strings.TrimSpace(node.Data) == "" {
		node = node.NextSibling
	}
	return node
}

func wrapPhrasingContentInParagraphs(node *xhtml.Node) {
	for child := node.FirstChild; child != nil; {
		nextSibling := child.NextSibling
		if !isParagraphPhrasingNode(child) {
			child = nextSibling
			continue
		}
		var fragment []*xhtml.Node
		for child != nil && isParagraphPhrasingNode(child) {
			nextSibling = child.NextSibling
			node.RemoveChild(child)
			fragment = append(fragment, child)
			child = nextSibling
		}
		for len(fragment) > 0 && isIgnorablePhrasingBoundaryNode(fragment[0]) {
			fragment = fragment[1:]
		}
		for len(fragment) > 0 && isIgnorablePhrasingBoundaryNode(fragment[len(fragment)-1]) {
			fragment = fragment[:len(fragment)-1]
		}
		if len(fragment) > 0 {
			p := &xhtml.Node{Type: xhtml.ElementNode, Data: "p"}
			for _, fragmentNode := range fragment {
				p.AppendChild(fragmentNode)
			}
			node.InsertBefore(p, nextSibling)
		}
		child = nextSibling
	}
}

func isWhitespaceNode(node *xhtml.Node) bool {
	return node != nil && node.Type == xhtml.TextNode && strings.TrimSpace(node.Data) == ""
}

func isIgnorablePhrasingBoundaryNode(node *xhtml.Node) bool {
	return isWhitespaceNode(node) || tagNameNode(node) == "br"
}

func removeComments(root *goquery.Selection) {
	root.Contents().Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		if node.Type == xhtml.CommentNode {
			removeNode(node)
			return
		}
		removeComments(s)
	})
}

func isPhrasingNode(node *xhtml.Node) bool {
	if node.Type == xhtml.TextNode {
		return true
	}
	if node.Type != xhtml.ElementNode {
		return false
	}
	return phrasingElement[tagNameNode(node)]
}

func isParagraphPhrasingNode(node *xhtml.Node) bool {
	if node.Type == xhtml.ElementNode && blockElement[tagNameNode(node)] {
		return false
	}
	return isPhrasingNode(node)
}

func simplifyNestedElements(root *goquery.Selection) {
	for changed := true; changed; {
		changed = false
		root.Find("div, section").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			node := s.Get(0)
			if node == nil || strings.HasPrefix(attr(s, "id"), "readability") {
				return true
			}
			if elementWithoutContent(s) {
				removeNode(node)
				changed = true
				return false
			}
			child := firstElementChild(node)
			if (nodeAttr(node, "id") == "content" || nodeAttr(node, "id") == "content-main") && child != nil &&
				(tagNameNode(child) == "article" || selectionForNode(child).Find("article").Length() > 0) {
				return true
			}
			if node.Parent != nil && (nodeAttr(node.Parent, "id") == "content" || nodeAttr(node.Parent, "id") == "content-main") && tagNameNode(child) == "article" {
				return true
			}
			if child != nil && nextElementSibling(child) == nil && (tagNameNode(child) == "div" || tagNameNode(child) == "section") {
				mergeMissingAttributes(child, node)
				replaceNode(node, child)
				changed = true
				return false
			}
			return true
		})
	}
}

func unwrapSingleCellTables(root *goquery.Selection) {
	root.Find("table").Each(func(_ int, s *goquery.Selection) {
		table := s.Get(0)
		if table == nil || table.Parent == nil {
			return
		}
		cells := s.Find("td, th")
		if cells.Length() != 1 {
			return
		}
		cell := cells.First().Get(0)
		if cell == nil {
			return
		}
		children := childNodes(cell)
		if isParagraphPhrasingFragment(children) {
			p := &xhtml.Node{Type: xhtml.ElementNode, Data: "p"}
			for _, child := range children {
				cell.RemoveChild(child)
				p.AppendChild(child)
			}
			table.Parent.InsertBefore(p, table)
		} else {
			for _, child := range children {
				cell.RemoveChild(child)
				table.Parent.InsertBefore(child, table)
			}
		}
		removeNode(table)
	})
}

func isParagraphPhrasingFragment(nodes []*xhtml.Node) bool {
	hasContent := false
	for _, node := range nodes {
		if isIgnorablePhrasingBoundaryNode(node) {
			continue
		}
		if !isParagraphPhrasingNode(node) {
			return false
		}
		hasContent = true
	}
	return hasContent
}

func mergeMissingAttributes(dst, src *xhtml.Node) {
	for _, attr := range src.Attr {
		replaced := false
		for i := range dst.Attr {
			if dst.Attr[i].Key == attr.Key {
				dst.Attr[i] = attr
				replaced = true
				break
			}
		}
		if !replaced {
			dst.Attr = append(dst.Attr, attr)
		}
	}
}
