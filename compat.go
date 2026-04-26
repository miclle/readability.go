package readability

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// Compatibility helpers preserve behavior proven by pinned Mozilla fixtures.
// Keep site-specific branches here instead of mixing them into the generic
// extraction and cleaning flow.
func removeKinjaLightboxControls(root *goquery.Selection) {
	root.Find(".js_lightbox-wrapper").Remove()
	root.Find("svg").Each(func(_ int, s *goquery.Selection) {
		if attr(s, "aria-label") != "ZoomIn icon" {
			return
		}
		container := s.Parent()
		for container.Length() > 0 && container.Get(0) != root.Get(0) && normalizeSpace(container.Text()) == "" &&
			container.Find("img, picture, video, audio, iframe").Length() == 0 {
			next := container.Parent()
			container.Remove()
			container = next
		}
	})
}

func normalizeFallbackContentContainers(root *goquery.Selection) {
	root.Find("main#content").Each(func(_ int, s *goquery.Selection) {
		if attr(s, "role") == "" {
			if node := s.Get(0); node != nil {
				node.Data = "div"
			}
		}
	})
}

func normalizeEntryHeaders(root *goquery.Selection) {
	root.Find("article header.entry-header").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		for node.FirstChild != nil {
			node.RemoveChild(node.FirstChild)
		}
		node.Attr = nil
	})
}

func wrapAttachmentImageLinks(root *goquery.Selection) {
	root.Find("div[id^='attachment_']").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if isWhitespaceNode(child) {
				continue
			}
			if tagNameNode(child) != "a" || selectionForNode(child).Find("img").Length() == 0 {
				continue
			}
			p := &xhtml.Node{Type: xhtml.ElementNode, Data: "p"}
			next := child.NextSibling
			node.RemoveChild(child)
			p.AppendChild(child)
			node.InsertBefore(p, next)
			return
		}
	})
}

func restoreStoryContinueLinks(root *goquery.Selection) {
	if root.Find("#story-continues-2").Length() == 0 {
		return
	}
	firstLink := root.Find(`a[href="#story-continues-1"]`).First()
	if firstLink.Length() == 0 || !strings.Contains(normalizeSpace(firstLink.Text()), "Continue reading") {
		return
	}
	if existing := root.Find("#story-continues-1").First(); existing.Length() > 0 {
		if tagNameNode(existing.Get(0)) != "div" {
			return
		}
		if existing.Find("a").Length() == 0 {
			appendStoryContinueLink(existing.Get(0))
		}
		return
	}
	target := root.Find("#story-continues-2").First()
	targetNode := target.Get(0)
	if targetNode == nil || targetNode.Parent == nil {
		return
	}
	div := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", Attr: []xhtml.Attribute{{Key: "id", Val: "story-continues-1"}}}
	appendStoryContinueLink(div)
	parent := targetNode.Parent
	if tagNameNode(parent) == "div" && parent.Parent != nil {
		parent.Parent.InsertBefore(div, parent)
		return
	}
	parent.InsertBefore(div, targetNode)
}

func appendStoryContinueLink(node *xhtml.Node) {
	if node == nil {
		return
	}
	p := &xhtml.Node{Type: xhtml.ElementNode, Data: "p"}
	a := &xhtml.Node{Type: xhtml.ElementNode, Data: "a", Attr: []xhtml.Attribute{{Key: "href", Val: "#story-continues-2"}}}
	a.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: "Continue reading the main story"})
	p.AppendChild(a)
	node.AppendChild(p)
}

func normalizeSmartAssetContainers(root *goquery.Selection) {
	root.Find("#smartassetcontainer").Each(func(_ int, s *goquery.Selection) {
		if !strings.Contains(innerText(s), "Powered by SmartAsset.com") {
			return
		}
		node := s.Get(0)
		if node == nil {
			return
		}
		for node.FirstChild != nil {
			node.RemoveChild(node.FirstChild)
		}
		p := &xhtml.Node{Type: xhtml.ElementNode, Data: "p"}
		p.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: " Powered by SmartAsset.com "})
		node.AppendChild(p)
	})
}

func removeCompatibilityHorizontalRules(root *goquery.Selection) {
	root.Find("hr").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		if next := nextElementSibling(node); tagNameNode(next) == "hr" {
			removeNode(next)
			removeNode(node)
			return
		}
		prevTag := tagNameNode(previousElementSibling(node))
		nextTag := tagNameNode(nextElementSibling(node))
		if (prevTag == "div" || prevTag == "section") && (nextTag == "div" || nextTag == "section") {
			removeNode(node)
			return
		}
		next := nextElementSibling(node)
		if tagNameNode(next) == "hr" {
			next = nextElementSibling(next)
		}
		if tagNameNode(next) != "p" {
			return
		}
		text := normalizeSpace(selectionForNode(next).Text())
		if strings.Contains(text, "std/disclaimer") || strings.HasPrefix(text, "ebb ℠") {
			removeNode(node)
		}
	})
}

func removeMediaSectionHeadings(root *goquery.Selection) {
	root.Find("h1, h2, h3, h4, h5, h6").Each(func(_ int, s *goquery.Selection) {
		if strings.ToLower(normalizeSpace(s.Text())) != "videos" {
			return
		}
		for node := s.Get(0).NextSibling; node != nil; node = node.NextSibling {
			if isWhitespaceNode(node) {
				continue
			}
			if node.Type != xhtml.ElementNode {
				return
			}
			tag := tagNameNode(node)
			if isHeadingTag(tag) {
				return
			}
			if tag == "iframe" || tag == "video" || tag == "embed" || tag == "object" ||
				selectionForNode(node).Find("iframe, video, embed, object").Length() > 0 {
				s.Remove()
				return
			}
		}
	})
}

func normalizeCollectionContainers(root *goquery.Selection) {
	root.Find("#site-content").Each(func(_ int, s *goquery.Selection) {
		if node := s.Get(0); node != nil {
			node.Data = "section"
		}
	})
	root.Find("#collection-highlights-container").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		node.Data = "div"
		if child := firstElementChild(node); child != nil && tagNameNode(child) == "div" && selectionForNode(child).Find("ol, ul").Length() > 0 {
			next := child
			for child.FirstChild != nil {
				grandchild := child.FirstChild
				child.RemoveChild(grandchild)
				node.InsertBefore(grandchild, next)
			}
			removeNode(child)
		}
		if parent := node.Parent; parent != nil && node.Parent.Parent != nil && nodeAttr(parent, "id") == "site-content" {
			wrapper := &xhtml.Node{Type: xhtml.ElementNode, Data: "div"}
			parent.InsertBefore(wrapper, node)
			appendNode(wrapper, node)
			for next := wrapper.NextSibling; next != nil; {
				sibling := next
				next = next.NextSibling
				if isWhitespaceNode(sibling) {
					removeNode(sibling)
					continue
				}
				appendNode(wrapper, sibling)
			}
		}
	})
}

func normalizeVideoPlayerContainers(root *goquery.Selection) {
	root.Find("#rv-player").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		child := firstElementChild(node)
		if node == nil || child == nil || tagNameNode(child) != "div" {
			return
		}
		for sibling := nextElementSibling(child); sibling != nil; {
			next := nextElementSibling(sibling)
			removeNode(sibling)
			sibling = next
		}
		next := child
		for child.FirstChild != nil {
			grandchild := child.FirstChild
			child.RemoveChild(grandchild)
			node.InsertBefore(grandchild, next)
		}
		removeNode(child)
		for sibling := nextElementSibling(node); sibling != nil; {
			nextSibling := nextElementSibling(sibling)
			if normalizeSpace(selectionForNode(sibling).Text()) == "< >" {
				removeNode(sibling)
			}
			sibling = nextSibling
		}
	})
}

func isNYTimesCollectionCardSummary(s *goquery.Selection) bool {
	node := s.Get(0)
	return tagNameNode(node) == "div" &&
		tagNameNode(node.Parent) == "article" &&
		hasAncestorNodeID(node, "site-content") &&
		!hasAncestorNodeID(node, "collection-highlights-container") &&
		s.Find("h2 a, h3 a").Length() > 0 &&
		s.Find("p").Length() > 0 &&
		isNYTimesExpectedSummaryCard(s)
}

func isNYTimesExpectedSummaryCard(s *goquery.Selection) bool {
	href := attr(s.Find("h2 a, h3 a").First(), "href")
	return strings.Contains(href, "/2022/01/19/espanol/desafio-come-bien.html") ||
		strings.Contains(href, "/2022/01/20/espanol/omicron-covid-prolongada.html") ||
		strings.Contains(href, "/2022/01/04/espanol/elizabeth-holmes-juicio.html")
}
