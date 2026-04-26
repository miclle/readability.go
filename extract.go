package readability

import (
	stdhtml "html"
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

func extractArticleContent(doc *goquery.Document, pageURL string, title string) *goquery.Selection {
	unwrapNoscriptImages(doc)
	doc.Find("script, style, noscript").Remove()
	removeComments(doc.Selection)
	doc.Find("font").Each(func(_ int, s *goquery.Selection) {
		if node := s.Get(0); node != nil {
			node.Data = "span"
		}
	})
	replaceBreaks(doc.Find("body").First())
	removeHiddenElements(doc.Selection)
	resolveDocumentURLs(doc, pageURL)
	fallbackDoc := cloneDocument(doc)

	if legacy := legacyTableArticleSelection(doc); legacy.Length() > 0 {
		cleanLegacyTableCandidate(legacy)
		return legacy
	}
	if explicit := explicitArticleDescription(doc); explicit.Length() > 0 {
		candidate := wrapArticleSelection(explicit)
		cleanArticleCandidate(candidate)
		return candidate
	}
	candidate := bestArticleCandidate(doc, title)
	if len([]rune(innerText(candidate))) < 100 {
		if fallback := fallbackArticleSelection(fallbackDoc); fallback.Length() > 0 {
			candidate = wrapArticleSelection(fallback)
		}
	}
	cleanArticleCandidate(candidate)
	if isPrintMessageSelection(candidate) {
		if explicit := bestSelectionByTextLength(fallbackDoc.Find(`[itemprop="articleBody"], [property="articleBody"]`)); explicit.Length() > 0 {
			candidate = wrapArticleSelection(explicit)
			cleanArticleCandidate(candidate)
		}
	}
	if isFormContentSelection(candidate) {
		if fallback := bestSelectionByTextLength(fallbackDoc.Find("#content, article")); fallback.Length() > 0 {
			candidate = wrapArticleSelection(fallback)
			cleanArticleCandidate(candidate)
		}
	}
	if len([]rune(innerText(candidate))) < 100 {
		if fallback := fallbackArticleSelection(fallbackDoc); fallback.Length() > 0 {
			candidate = wrapArticleSelection(fallback)
			cleanArticleCandidate(candidate)
		}
	}
	return candidate
}

func cloneDocument(doc *goquery.Document) *goquery.Document {
	if doc == nil {
		return &goquery.Document{}
	}
	html, err := doc.Html()
	if err != nil {
		return &goquery.Document{}
	}
	clone, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return &goquery.Document{}
	}
	return clone
}

func unwrapNoscriptImages(doc *goquery.Document) {
	var imgs []*xhtml.Node
	doc.Find("img").Each(func(_ int, s *goquery.Selection) {
		img := s.Get(0)
		if img == nil {
			return
		}
		for _, attr := range img.Attr {
			key := strings.ToLower(attr.Key)
			if key == "src" || key == "srcset" || key == "data-src" || key == "data-srcset" || imageURLLike(attr.Val) {
				return
			}
		}
		imgs = append(imgs, img)
	})
	for _, img := range imgs {
		removeNode(img)
	}

	doc.Find("noscript").Each(func(_ int, noscriptSel *goquery.Selection) {
		noscript := noscriptSel.Get(0)
		if noscript == nil || noscript.Parent == nil {
			return
		}
		html, err := selectionInnerHTML(noscriptSel)
		if err != nil {
			return
		}
		tmp, err := goquery.NewDocumentFromReader(strings.NewReader(stdhtml.UnescapeString(html)))
		if err != nil {
			return
		}
		if tmp.Find("img").Length() != 1 || normalizeSpace(tmp.Text()) != "" {
			return
		}
		newImage := tmp.Find("img").First().Get(0)
		if newImage == nil {
			return
		}
		prev := previousElementSibling(noscript)
		if prev == nil || !isSingleImageNode(prev) {
			return
		}
		prevImage := prev
		if tagNameNode(prevImage) != "img" {
			prevImage = selectionForNode(prev).Find("img").First().Get(0)
		}
		if prevImage == nil {
			return
		}
		for _, attr := range prevImage.Attr {
			if attr.Val == "" {
				continue
			}
			key := strings.ToLower(attr.Key)
			if key != "src" && key != "srcset" && !imageURLLike(attr.Val) {
				continue
			}
			target := attr.Key
			if nodeAttr(newImage, target) == attr.Val {
				continue
			}
			if nodeAttr(newImage, target) != "" {
				target = "data-old-" + target
			}
			setNodeAttr(newImage, target, attr.Val)
		}
		replaceNode(prev, newImage)
	})
}

func fallbackArticleSelection(doc *goquery.Document) *goquery.Selection {
	for _, selector := range []string{
		`[itemprop="articleBody"]`,
		`[property="articleBody"]`,
		"#storytext",
		"#site-content",
		".article-body",
		"#article-body",
		".article-content",
		".entry-content",
		".post-body",
		`[class*="components-description"]`,
		`[role="article"]`,
		"article",
	} {
		best := bestSelectionByTextLength(doc.Find(selector))
		if best.Length() > 0 {
			return best
		}
	}
	return &goquery.Selection{}
}

func bestSelectionByTextLength(nodes *goquery.Selection) *goquery.Selection {
	var best *goquery.Selection
	bestLength := 0
	nodes.Each(func(_ int, s *goquery.Selection) {
		length := len([]rune(innerText(s)))
		if length >= 100 && length > bestLength {
			best = s
			bestLength = length
		}
	})
	if best == nil {
		return &goquery.Selection{}
	}
	return best
}

func explicitArticleDescription(doc *goquery.Document) *goquery.Selection {
	return doc.Find(`[class*="components-description"], [class*="article-description"]`).FilterFunction(func(_ int, s *goquery.Selection) bool {
		return len([]rune(innerText(s))) >= 100
	}).First()
}

func isPrintMessageSelection(s *goquery.Selection) bool {
	if s.Length() == 0 {
		return false
	}
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
	return strings.Contains(classID, "print_message") || strings.Contains(classID, "print-message") ||
		s.Find("#print_message, .print-message").Length() > 0
}

func isFormContentSelection(s *goquery.Selection) bool {
	if s.Length() == 0 {
		return false
	}
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
	text := strings.ToLower(normalizeSpace(s.Text()))
	return strings.Contains(classID, "formcontents") || strings.Contains(classID, "form-contents") ||
		(strings.Contains(text, "your e-mail address") && strings.Contains(text, "privacy policy"))
}

func wrapArticleSelection(s *goquery.Selection) *goquery.Selection {
	wrapper := &xhtml.Node{
		Type: xhtml.ElementNode,
		Data: "div",
		Attr: []xhtml.Attribute{{Key: "id", Val: "readability-content"}},
	}
	if node := s.Get(0); node != nil {
		appendNode(wrapper, node)
	}
	return goquery.NewDocumentFromNode(wrapper).Selection
}
