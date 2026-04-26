package readability

import (
	stdhtml "html"
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

func extractArticleContent(doc *goquery.Document, pageURL string, title string, cfg parserConfig) *goquery.Selection {
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
		cleanLegacyTableCandidate(legacy, cfg)
		return legacy
	}
	if explicit := explicitArticleDescription(doc); explicit.Length() > 0 {
		candidate := wrapArticleSelection(explicit)
		cleanArticleCandidateConfig(candidate, cfg)
		return candidate
	}
	candidate := bestArticleCandidate(doc, title, cfg)
	if len([]rune(innerText(candidate))) < 100 {
		if fallback := fallbackArticleSelection(fallbackDoc); fallback.Length() > 0 {
			candidate = wrapArticleSelection(fallback)
		} else if relaxed := relaxedArticleSelection(fallbackDoc, title, cfg); relaxed.Length() > 0 {
			candidate = relaxed
			return candidate
		}
	}
	cleanArticleCandidateConfig(candidate, cfg)
	if isPrintMessageSelection(candidate) {
		if explicit := bestSelectionByTextLength(fallbackDoc.Find(`[itemprop="articleBody"], [property="articleBody"]`)); explicit.Length() > 0 {
			candidate = wrapArticleSelection(explicit)
			cleanArticleCandidateConfig(candidate, cfg)
		}
	}
	if isFormContentSelection(candidate) {
		if fallback := bestSelectionByTextLength(fallbackDoc.Find("#content, article")); fallback.Length() > 0 {
			candidate = wrapArticleSelection(fallback)
			cleanArticleCandidateConfig(candidate, cfg)
		}
	}
	if len([]rune(innerText(candidate))) < 100 {
		if fallback := fallbackArticleSelection(fallbackDoc); fallback.Length() > 0 {
			candidate = wrapArticleSelection(fallback)
			cleanArticleCandidateConfig(candidate, cfg)
		}
	}
	return candidate
}

func relaxedArticleSelection(doc *goquery.Document, title string, cfg parserConfig) *goquery.Selection {
	attempts := []struct {
		scoring articleScoringOptions
		clean   articleCleanOptions
	}{
		{
			scoring: articleScoringOptions{StripUnlikely: false, WeightClasses: true, Config: cfg},
			clean:   articleCleanOptions{StripUnlikely: false, WeightClasses: true, CleanConditionally: true, Config: cfg},
		},
		{
			scoring: articleScoringOptions{StripUnlikely: false, WeightClasses: false, Config: cfg},
			clean:   articleCleanOptions{StripUnlikely: false, WeightClasses: false, CleanConditionally: true, Config: cfg},
		},
		{
			scoring: articleScoringOptions{StripUnlikely: false, WeightClasses: false, Config: cfg},
			clean:   articleCleanOptions{StripUnlikely: false, WeightClasses: false, CleanConditionally: false, Config: cfg},
		},
	}
	for _, attempt := range attempts {
		relaxedDoc := cloneDocument(doc)
		candidate := bestArticleCandidateWithOptions(relaxedDoc, title, attempt.scoring)
		if len([]rune(innerText(candidate))) < 100 {
			continue
		}
		cleanArticleCandidateWithOptions(candidate, attempt.clean)
		if len([]rune(innerText(candidate))) >= 100 {
			return candidate
		}
	}
	return &goquery.Selection{}
}

func cloneDocument(doc *goquery.Document) *goquery.Document {
	if doc == nil {
		return &goquery.Document{}
	}
	root := doc.Get(0)
	if root == nil {
		return &goquery.Document{}
	}
	return goquery.NewDocumentFromNode(cloneNode(root))
}

// cloneNode returns a deep copy of n, including its descendants. Parent and
// sibling pointers on the returned root are nil; descendants are linked
// internally so they form an independent tree from the source.
func cloneNode(n *xhtml.Node) *xhtml.Node {
	if n == nil {
		return nil
	}
	clone := &xhtml.Node{
		Type:      n.Type,
		DataAtom:  n.DataAtom,
		Data:      n.Data,
		Namespace: n.Namespace,
	}
	if len(n.Attr) > 0 {
		clone.Attr = make([]xhtml.Attribute, len(n.Attr))
		copy(clone.Attr, n.Attr)
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		childClone := cloneNode(child)
		childClone.Parent = clone
		if clone.FirstChild == nil {
			clone.FirstChild = childClone
		} else {
			clone.LastChild.NextSibling = childClone
			childClone.PrevSibling = clone.LastChild
		}
		clone.LastChild = childClone
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
		if dir := directionForCandidate(node); dir != "" {
			setNodeAttr(wrapper, "dir", dir)
		}
		appendNode(wrapper, node)
	}
	return goquery.NewDocumentFromNode(wrapper).Selection
}
