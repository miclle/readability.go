package readability

import (
	stdhtml "html"
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// extractArticleContent is the parser entry point that turns a parsed
// HTML document into the final article subtree. The function follows a
// strict order:
//   1. Pre-cleanup: remove scripts/styles/comments, replace <font> with
//      <span>, collapse <br> runs into <p>, drop hidden elements, and
//      resolve relative URLs. fallbackDoc is captured before the
//      destructive scoring pass so retries can score against pristine
//      markup.
//   2. Legacy table fixtures: when the document looks like a 90s
//      newspaper layout (single article inside a table), bypass scoring.
//   3. Explicit articleBody descriptions: certain CMS templates expose
//      `[class*=components-description]` blocks that already are the
//      article and don't survive scoring well.
//   4. Standard scoring via bestArticleCandidate.
//   5. Multiple fallbacks if the candidate is too short, looks like a
//      print-message page, or matches the "form contents" footer
//      pattern. Each fallback selects from the pristine fallbackDoc.
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

// relaxedArticleSelection is the fallback ladder when the strict
// scoring pass produced too little content (< 100 chars). Each rung
// progressively relaxes the heuristics on a fresh clone of the
// fallbackDoc so cleanup state from earlier rungs cannot leak:
//   1. drop StripUnlikely (keep class weighting and conditional cleanup)
//   2. also drop class weighting
//   3. also drop conditional cleanup
// The first rung that yields >= 100 chars wins. Returns an empty
// selection when even the most permissive rung fails.
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

// unwrapNoscriptImages handles the common pattern where a site emits a
// placeholder <img> for non-JS clients followed by a <noscript> wrapper
// containing the real image. Two passes:
//  1. drop bare <img> elements that have no usable src/srcset/data-src
//     and whose remaining attributes do not look URL-shaped (otherwise
//     scoring would weight a broken placeholder as image content);
//  2. for each <noscript> whose only child is a single <img> and has no
//     surrounding text, copy that <img>'s URL-shaped attributes onto the
//     immediately preceding placeholder image, then replace the
//     placeholder with the noscript-supplied <img>. This lets the
//     scorer treat the lazily-loaded image as the real image.
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
