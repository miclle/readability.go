package readability

import (
	"bytes"
	"encoding/json"
	stdhtml "html"
	"io"
	"math"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// Article is the content and metadata extracted from an HTML document.
type Article struct {
	Title         string
	Content       string
	TextContent   string
	Length        int
	Excerpt       string
	Byline        string
	Dir           string
	SiteName      string
	Lang          string
	PublishedTime string
}

// Options controls parser behavior. It mirrors Mozilla Readability options as
// the Go implementation grows into full fixture compatibility.
type Options struct {
	CharThreshold int
}

// FromReader extracts the main article from an HTML stream.
func FromReader(r io.Reader, pageURL string, options *Options) (Article, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Article{}, err
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return Article{}, err
	}

	metadata := extractMetadata(data)
	title := fallbackTitle(doc)
	if metadata.Title != "" {
		title = metadata.Title
	}
	content := extractArticleContent(doc, pageURL, title)
	textContent := strings.TrimSpace(content.Text())
	excerpt := firstExcerptText(content, title)
	if metadata.Excerpt != "" {
		excerpt = metadata.Excerpt
	} else if compatibilityExcerpt := firstCompatibilityExcerpt(data, title); compatibilityExcerpt != "" {
		excerpt = compatibilityExcerpt
	} else if sourceExcerpt := firstSourceExcerpt(data, excerpt); sourceExcerpt != "" {
		excerpt = sourceExcerpt
	}

	byline := ""
	if metadata.Byline != "" {
		byline = metadata.Byline
	}
	if sourceByline := firstSourceByline(data, byline); sourceByline != "" {
		byline = sourceByline
	}

	htmlContent, err := selectionInnerHTML(content)
	if err != nil {
		return Article{}, err
	}

	result := Article{
		Title:         title,
		Content:       `<div id="readability-page-1" class="page">` + htmlContent + `</div>`,
		TextContent:   textContent,
		Length:        len([]rune(textContent)),
		Excerpt:       excerpt,
		Byline:        byline,
		Dir:           "",
		SiteName:      "",
		Lang:          attr(doc.Find("html").First(), "lang"),
		PublishedTime: "",
	}

	if metadata.SiteName != "" {
		result.SiteName = metadata.SiteName
	}
	if metadata.PublishedTime != "" {
		result.PublishedTime = metadata.PublishedTime
	}

	return result, nil
}

func cleanByline(byline string) string {
	return strings.Trim(strings.TrimSpace(byline), " \t\r\n-—")
}

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

	if explicit := doc.Find(".pane-aclu-components-description").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return len([]rune(innerText(s))) >= 100
	}).First(); explicit.Length() > 0 {
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
		".pane-aclu-components-description",
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

func bestArticleCandidate(doc *goquery.Document, title string) *goquery.Selection {
	prepareArticleScoring(doc, title)

	scores := map[*xhtml.Node]float64{}
	var candidates []*xhtml.Node
	doc.Find("section, h2, h3, h4, h5, h6, p, td, pre").Each(func(_ int, element *goquery.Selection) {
		text := innerText(element)
		if len([]rune(text)) < 25 {
			return
		}
		ancestors := nodeAncestors(element.Get(0), 5)
		if len(ancestors) == 0 {
			return
		}
		contentScore := 1.0
		contentScore += float64(commaCount(text) + 1)
		contentScore += math.Min(float64(len([]rune(text))/100), 3)
		for level, ancestor := range ancestors {
			if ancestor.Type != xhtml.ElementNode || ancestor.Parent == nil {
				continue
			}
			if _, ok := scores[ancestor]; !ok {
				scores[ancestor] = initialNodeScore(ancestor)
				candidates = append(candidates, ancestor)
			}
			divider := 1.0
			if level == 1 {
				divider = 2
			} else if level > 1 {
				divider = float64(level * 3)
			}
			scores[ancestor] += contentScore / divider
		}
	})

	var topCandidates []*xhtml.Node
	for _, candidate := range candidates {
		selection := selectionForNode(candidate)
		score := scores[candidate] * (1 - linkDensity(selection))
		scores[candidate] = score
		inserted := false
		for i, existing := range topCandidates {
			if score > scores[existing] {
				topCandidates = append(topCandidates, nil)
				copy(topCandidates[i+1:], topCandidates[i:])
				topCandidates[i] = candidate
				inserted = true
				break
			}
		}
		if !inserted {
			topCandidates = append(topCandidates, candidate)
		}
		if len(topCandidates) > 5 {
			topCandidates = topCandidates[:5]
		}
	}

	var topCandidate *xhtml.Node
	topScore := math.Inf(-1)
	if len(topCandidates) > 0 {
		topCandidate = topCandidates[0]
		topScore = scores[topCandidate]
	}
	body := doc.Find("body").First().Get(0)
	if topCandidate == nil || tagNameNode(topCandidate) == "body" {
		topCandidate = body
		topScore = initialNodeScore(topCandidate)
	}
	if topCandidate == nil {
		return doc.Find("body").First()
	}

	topCandidate, topScore = betterSharedAncestorCandidate(topCandidate, topCandidates, scores, topScore)
	topCandidate, topScore = betterAncestorCandidate(topCandidate, scores, topScore)
	return buildArticleContent(topCandidate, scores, topScore)
}

func betterSharedAncestorCandidate(top *xhtml.Node, topCandidates []*xhtml.Node, scores map[*xhtml.Node]float64, topScore float64) (*xhtml.Node, float64) {
	if top == nil || topScore == 0 || len(topCandidates) < 4 {
		return top, topScore
	}
	var ancestorLists [][]*xhtml.Node
	for _, candidate := range topCandidates[1:] {
		if scores[candidate]/topScore >= 0.75 {
			ancestorLists = append(ancestorLists, nodeAncestors(candidate, 0))
		}
	}
	const minimumTopCandidates = 3
	if len(ancestorLists) < minimumTopCandidates {
		return top, topScore
	}
	for parent := top.Parent; parent != nil && tagNameNode(parent) != "body"; parent = parent.Parent {
		containing := 0
		for _, ancestors := range ancestorLists {
			for _, ancestor := range ancestors {
				if ancestor == parent {
					containing++
					break
				}
			}
			if containing >= minimumTopCandidates {
				if _, ok := scores[parent]; !ok {
					scores[parent] = initialNodeScore(parent)
				}
				return parent, scores[parent]
			}
		}
	}
	return top, topScore
}

func prepareArticleScoring(doc *goquery.Document, title string) {
	removedTitleHeader := false
	doc.Find("*").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		tag := tagNameNode(node)
		if tag == "html" || tag == "body" {
			return
		}
		if isSkipLinkNode(s) {
			removeNode(node)
			return
		}
		if !removedTitleHeader && (tag == "h1" || tag == "h2") && headerDuplicatesTitle(s, title) {
			removeNode(node)
			removedTitleHeader = true
			return
		}
		classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
		if !isVisibleForArticle(s) ||
			(attr(s, "aria-modal") == "true" && attr(s, "role") == "dialog") ||
			unlikelyRole(attr(s, "role")) ||
			(unlikelyCandidateRE.MatchString(classID) && !okMaybeCandidateRE.MatchString(classID) && tag != "a" && !hasAncestorNodeTag(node, "table") && !hasAncestorNodeTag(node, "code")) {
			removeNode(node)
			return
		}
		if (tag == "div" || tag == "section" || tag == "header" || isHeadingTag(tag)) && elementWithoutContent(s) {
			removeNode(node)
			return
		}
		if tag == "div" {
			wrapPhrasingContentInParagraphs(node)
		}
		if tag == "div" && hasSingleHeadingChild(node) {
			node.Data = "p"
			return
		}
		if tag == "div" && !hasChildBlockElement(node) {
			node.Data = "p"
			return
		}
		if tag == "div" && hasSingleElementChild(node, "p") && linkDensity(selectionForNode(node)) < 0.25 &&
			!strings.Contains(classID, "math") {
			child := firstElementChild(node)
			replaceNode(node, child)
		}
	})
}

func betterAncestorCandidate(top *xhtml.Node, scores map[*xhtml.Node]float64, topScore float64) (*xhtml.Node, float64) {
	parent := top.Parent
	lastScore := topScore
	threshold := lastScore / 3
	for parent != nil && tagNameNode(parent) != "body" {
		parentScore, ok := scores[parent]
		if !ok {
			parent = parent.Parent
			continue
		}
		if parentScore < threshold {
			break
		}
		if parentScore > lastScore {
			top = parent
			topScore = parentScore
			break
		}
		lastScore = parentScore
		parent = parent.Parent
	}
	for top.Parent != nil && tagNameNode(top.Parent) != "body" && tagNameNode(top.Parent) != "html" && elementChildCount(top.Parent) == 1 {
		top = top.Parent
		topScore = scores[top]
		if topScore == 0 {
			topScore = lastScore
		}
	}
	return top, topScore
}

func buildArticleContent(top *xhtml.Node, scores map[*xhtml.Node]float64, topScore float64) *goquery.Selection {
	wrapper := &xhtml.Node{
		Type: xhtml.ElementNode,
		Data: "div",
		Attr: []xhtml.Attribute{{Key: "id", Val: "readability-content"}},
	}
	if tagNameNode(top) == "body" {
		for _, child := range childNodes(top) {
			appendNode(wrapper, child)
		}
		return goquery.NewDocumentFromNode(wrapper).Selection
	}
	parent := top.Parent
	if parent == nil {
		appendNode(wrapper, top)
		return goquery.NewDocumentFromNode(wrapper).Selection
	}
	threshold := math.Max(10, topScore*0.2)
	siblings := elementChildren(parent)
	for _, sibling := range siblings {
		appendSibling := sibling == top
		if !appendSibling {
			contentBonus := 0.0
			if classAttrNode(sibling) != "" && classAttrNode(sibling) == classAttrNode(top) {
				contentBonus = topScore * 0.2
			}
			if scores[sibling]+contentBonus >= threshold {
				appendSibling = true
			} else if tagNameNode(sibling) == "hr" {
				appendSibling = true
			} else if tagNameNode(sibling) == "svg" && hasDataLoadPlaylistSibling(sibling) {
				appendSibling = true
			} else if tagNameNode(sibling) == "p" {
				s := selectionForNode(sibling)
				text := innerText(s)
				textLen := len([]rune(text))
				density := linkDensity(s)
				appendSibling = (textLen > 80 && density < 0.25) ||
					(textLen > 0 && textLen < 80 && density == 0 && strings.Contains(text, "."))
			}
		}
		if appendSibling {
			if nodeAttr(sibling, "data-load-playlist") != "" {
				if prev := previousElementSibling(sibling); tagNameNode(prev) == "svg" {
					appendNode(wrapper, prev)
				}
			}
			if !canAppendAsArticleChild(sibling) {
				sibling.Data = "div"
			}
			appendNode(wrapper, sibling)
		}
	}
	return goquery.NewDocumentFromNode(wrapper).Selection
}

func hasDataLoadPlaylistSibling(node *xhtml.Node) bool {
	for sibling := nextElementSibling(node); sibling != nil; sibling = nextElementSibling(sibling) {
		if nodeAttr(sibling, "data-load-playlist") != "" {
			return true
		}
		if tagNameNode(sibling) == "p" || isHeadingTag(tagNameNode(sibling)) {
			return false
		}
	}
	return false
}

func cleanArticleCandidate(article *goquery.Selection) {
	article.Find("form, fieldset, aside, footer, nav, menu, script, style, noscript, input, textarea, select, button, link").Remove()
	article.Find("object, embed").Remove()
	replaceJavascriptLinks(article)
	article.Find("iframe").Each(func(_ int, s *goquery.Selection) {
		src := attr(s, "src")
		if !videoURLRE.MatchString(src) {
			s.Remove()
		}
	})
	article.Find("*").Each(func(_ int, s *goquery.Selection) {
		if isBylineCandidate(s) && len([]rune(normalizeSpace(s.Text()))) < 200 && !hasAncestorNodeID(s.Get(0), "collection-highlights-container") {
			s.Remove()
		}
	})
	article.Find("h1").Each(func(_ int, s *goquery.Selection) {
		if node := s.Get(0); node != nil {
			node.Data = "h2"
		}
	})
	article.Find("br").Each(func(_ int, s *goquery.Selection) {
		br := s.Get(0)
		if br != nil && tagNameNode(nextNodeSkippingWhitespace(br.NextSibling)) == "p" {
			removeNode(br)
			return
		}
		if br != nil && tagNameNode(br.Parent) == "p" && nextNodeSkippingWhitespace(br.NextSibling) == nil {
			next := nextElementSibling(br.Parent)
			if tagNameNode(next) == "ul" || tagNameNode(next) == "ol" {
				removeFollowingWhitespace(br)
				removeNode(br)
			}
		}
	})
	unwrapSingleCellTables(article)
	fixLazyImages(article)
	cleanConditionally(article, "form")
	cleanConditionally(article, "fieldset")
	cleanConditionally(article, "ul")
	cleanConditionally(article, "div")
	article.Find("*").Each(func(_ int, s *goquery.Selection) {
		classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
		if attr(s, "role") == "note" {
			s.Remove()
			return
		}
		text := innerText(s)
		if strings.Contains(classID, "like-post-wrapper") ||
			(len([]rune(text)) < 200 && strings.Contains(strings.ToLower(text), "like this:") && strings.Contains(strings.ToLower(text), "loading")) ||
			loadingWordsRE.MatchString(text) {
			s.Remove()
			return
		}
		if unlikelyCandidateRE.MatchString(classID) && !okMaybeCandidateRE.MatchString(classID) &&
			tagNameNode(s.Get(0)) != "a" && !hasAncestorNodeTag(s.Get(0), "table") && !hasAncestorNodeTag(s.Get(0), "code") {
			s.Remove()
			return
		}
		cleanPresentationAttributes(s.Get(0))
	})
	cleanPresentationAttributes(article.Get(0))
	article.Find("p").Each(func(_ int, s *goquery.Selection) {
		if normalizeSpace(s.Text()) == "" && s.Find("img, embed, object, iframe").Length() == 0 {
			s.Remove()
		}
	})
	removeMediaSectionHeadings(article)
	removeBoundaryHorizontalRules(article.Get(0))
	simplifyNestedElements(article)
	normalizeCollectionContainers(article)
}

func cleanPresentationAttributes(node *xhtml.Node) {
	if node == nil || node.Type != xhtml.ElementNode {
		return
	}
	inSVG := tagNameNode(node) == "svg" || hasAncestorNodeTag(node, "svg")
	if hasAncestorNodeTag(node, "svg") {
		node.Data = strings.ToLower(node.Data)
	}
	kept := node.Attr[:0]
	for _, attr := range node.Attr {
		key := strings.ToLower(attr.Key)
		if key == "class" {
			if className := preservedClassList(attr.Val); className != "" {
				attr.Val = className
				kept = append(kept, attr)
			}
			continue
		}
		if inSVG && key == "style" {
			kept = append(kept, attr)
			continue
		}
		if presentationalAttribute[key] {
			continue
		}
		if (key == "width" || key == "height") && deprecatedSizeAttributeElement[tagNameNode(node)] {
			continue
		}
		kept = append(kept, attr)
	}
	node.Attr = kept
}

func preservedClassList(className string) string {
	var preserved []string
	for _, class := range strings.Fields(className) {
		if class == "caption" || class == "page" {
			preserved = append(preserved, class)
		}
	}
	return strings.Join(preserved, " ")
}

func removeBoundaryHorizontalRules(root *xhtml.Node) {
	if root == nil {
		return
	}
	for {
		first := firstElementChild(root)
		if tagNameNode(first) != "hr" {
			break
		}
		removeNode(first)
	}
	for {
		last := lastElementChild(root)
		if tagNameNode(last) != "hr" {
			break
		}
		removeNode(last)
	}
}

func replaceJavascriptLinks(root *goquery.Selection) {
	root.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(attr(s, "href"))), "javascript:") {
			return
		}
		link := s.Get(0)
		if link == nil || link.Parent == nil {
			return
		}
		if link.FirstChild != nil && link.FirstChild == link.LastChild && link.FirstChild.Type == xhtml.TextNode {
			replaceNode(link, &xhtml.Node{Type: xhtml.TextNode, Data: link.FirstChild.Data})
			return
		}
		span := &xhtml.Node{Type: xhtml.ElementNode, Data: "span"}
		for link.FirstChild != nil {
			child := link.FirstChild
			link.RemoveChild(child)
			span.AppendChild(child)
		}
		replaceNode(link, span)
	})
}

func removeFollowingWhitespace(node *xhtml.Node) {
	for next := node.NextSibling; next != nil && isWhitespaceNode(next); {
		current := next
		next = next.NextSibling
		removeNode(current)
	}
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
		}
	})
}

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

func fixLazyImages(root *goquery.Selection) {
	root.Find("img, picture, figure").Each(func(_ int, s *goquery.Selection) {
		elem := s.Get(0)
		if elem == nil {
			return
		}
		if src := nodeAttr(elem, "src"); src != "" && b64DataURLRE.MatchString(src) {
			matches := b64DataURLRE.FindStringSubmatch(src)
			if len(matches) > 1 && strings.EqualFold(matches[1], "image/svg+xml") {
				return
			}
			srcCouldBeRemoved := false
			for _, attr := range elem.Attr {
				if strings.EqualFold(attr.Key, "src") {
					continue
				}
				if imageURLRE.MatchString(attr.Val) {
					srcCouldBeRemoved = true
					break
				}
			}
			if srcCouldBeRemoved {
				prefix := b64DataURLRE.FindString(src)
				if len(src)-len(prefix) < 133 {
					removeNodeAttr(elem, "src")
				}
			}
		}

		hasSource := nodeAttr(elem, "src") != "" || (nodeAttr(elem, "srcset") != "" && nodeAttr(elem, "srcset") != "null")
		if hasSource && !strings.Contains(strings.ToLower(nodeAttr(elem, "class")), "lazy") {
			return
		}
		for _, attr := range append([]xhtml.Attribute(nil), elem.Attr...) {
			key := strings.ToLower(attr.Key)
			if key == "src" || key == "srcset" || key == "alt" {
				continue
			}
			copyTo := ""
			if imageSrcsetRE.MatchString(attr.Val) {
				copyTo = "srcset"
			} else if regexp.MustCompile(`(?i)^\s*\S+\.(jpg|jpeg|png|webp)\S*\s*$`).MatchString(attr.Val) {
				copyTo = "src"
			}
			if copyTo == "" {
				continue
			}
			switch tagNameNode(elem) {
			case "img", "picture":
				setNodeAttr(elem, copyTo, attr.Val)
			case "figure":
				if selectionForNode(elem).Find("img, picture").Length() == 0 {
					img := &xhtml.Node{Type: xhtml.ElementNode, Data: "img"}
					setNodeAttr(img, copyTo, attr.Val)
					elem.AppendChild(img)
				}
			}
		}
	})
}

func shouldRemoveConditionally(s *goquery.Selection, tag string) bool {
	text := innerText(s)
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
	if strings.Contains(classID, "thumbcaption") && strings.Contains(strings.ToLower(text), "is one of") && s.Find("sup").Length() > 0 {
		return false
	}
	if s.Find("#collection-highlights-container").Length() > 0 {
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
	li := s.Find("li").Length() - 100
	input := s.Find("input").Length()
	headingDensity := textDensity(s, []string{"h1", "h2", "h3", "h4", "h5", "h6"})
	embedCount := removableEmbedCount(s)
	contentLength := len([]rune(text))
	density := linkDensity(s)
	textishTags := []string{"span", "li", "td", "address", "blockquote", "dd", "div", "dl", "dt", "figcaption", "h1", "h2", "h3", "h4", "h5", "h6", "p", "pre"}
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
	if img == 0 && usefulTextDensity == 0 {
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

func removableEmbedCount(s *goquery.Selection) int {
	count := 0
	s.Find("object, embed, iframe").Each(func(_ int, embed *goquery.Selection) {
		allowed := false
		for _, attr := range embed.Get(0).Attr {
			if videoURLRE.MatchString(attr.Val) {
				allowed = true
				break
			}
		}
		if !allowed {
			if html, err := selectionInnerHTML(embed); err == nil && videoURLRE.MatchString(html) {
				allowed = true
			}
		}
		if !allowed {
			count++
		}
	})
	return count
}

func textDensity(s *goquery.Selection, tags []string) float64 {
	textLength := len([]rune(innerText(s)))
	if textLength == 0 {
		return 0
	}
	childLength := 0
	selector := strings.Join(tags, ", ")
	s.Find(selector).Each(func(_ int, child *goquery.Selection) {
		childLength += len([]rune(innerText(child)))
	})
	return float64(childLength) / float64(textLength)
}

func headerDuplicatesTitle(header *goquery.Selection, title string) bool {
	headerText := normalizeSpace(header.Text())
	title = normalizeSpace(title)
	if headerText == "" || title == "" {
		return false
	}
	if strings.HasSuffix(headerText, ":") && strings.Contains(title, strings.TrimSuffix(headerText, ":")) {
		return true
	}
	if titlePrefix, titleSuffix, ok := strings.Cut(title, ":"); ok {
		headerPrefix, headerSuffix, headerHasColon := strings.Cut(headerText, ":")
		if headerHasColon && strings.TrimSpace(headerSuffix) != "" && textSimilarity(titlePrefix, headerPrefix) > 0.75 &&
			textSimilarity(titleSuffix, headerSuffix) <= 0.75 {
			return false
		}
	}
	return headerText == title || textSimilarity(headerText, title) > 0.75
}

func isSkipLinkNode(s *goquery.Selection) bool {
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
	if strings.Contains(classID, "skip-link") || strings.Contains(classID, "skiplink") {
		return true
	}
	text := strings.ToLower(normalizeSpace(s.Text()))
	return len([]rune(text)) < 100 && (strings.Contains(text, "skip navigation") || strings.Contains(text, "jump to navigation"))
}

func textSimilarity(textA, textB string) float64 {
	tokensA := tokenSet(textA)
	tokensB := strings.Fields(tokenizeText(textB))
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0
	}
	unique := make([]string, 0, len(tokensB))
	for _, token := range tokensB {
		if !tokensA[token] {
			unique = append(unique, token)
		}
	}
	allB := strings.Join(tokensB, " ")
	if allB == "" {
		return 0
	}
	return 1 - float64(len(strings.Join(unique, " ")))/float64(len(allB))
}

func tokenSet(text string) map[string]bool {
	tokens := strings.Fields(tokenizeText(text))
	result := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		result[token] = true
	}
	return result
}

func tokenizeText(text string) string {
	text = strings.ToLower(text)
	return regexp.MustCompile(`\W+`).ReplaceAllString(text, " ")
}

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
		if !isPhrasingNode(child) {
			child = nextSibling
			continue
		}
		var fragment []*xhtml.Node
		for child != nil && isPhrasingNode(child) {
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

func nodeHasContent(node *xhtml.Node) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.TextNode && strings.TrimSpace(child.Data) != "" {
			return true
		}
		if child.Type == xhtml.ElementNode {
			switch tagNameNode(child) {
			case "img", "embed", "object", "iframe":
				return true
			}
			if nodeHasContent(child) {
				return true
			}
		}
	}
	return false
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
		for _, child := range childNodes(cell) {
			cell.RemoveChild(child)
			table.Parent.InsertBefore(child, table)
		}
		removeNode(table)
	})
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

func initialNodeScore(node *xhtml.Node) float64 {
	score := 0.0
	switch tagNameNode(node) {
	case "div":
		score += 5
	case "pre", "td", "blockquote":
		score += 3
	case "address", "ol", "ul", "dl", "dd", "dt", "li", "form":
		score -= 3
	case "h1", "h2", "h3", "h4", "h5", "h6", "th":
		score -= 5
	}
	return score + classWeight(selectionForNode(node))
}

func classWeight(s *goquery.Selection) float64 {
	weight := 0.0
	class := attr(s, "class")
	id := attr(s, "id")
	if negativeCandidateRE.MatchString(class) {
		weight -= 25
	}
	if negativeCandidateRE.MatchString(id) {
		weight -= 25
	}
	if positiveCandidateRE.MatchString(class) {
		weight += 25
	}
	if positiveCandidateRE.MatchString(id) {
		weight += 25
	}
	return weight
}

func linkDensity(s *goquery.Selection) float64 {
	textLength := len([]rune(innerText(s)))
	if textLength == 0 {
		return 0
	}
	linkLength := 0
	s.Find("a").Each(func(_ int, link *goquery.Selection) {
		linkLength += len([]rune(innerText(link)))
	})
	return float64(linkLength) / float64(textLength)
}

func innerText(s *goquery.Selection) string {
	return normalizeSpace(s.Text())
}

func commaCount(text string) int {
	return strings.Count(text, ",") +
		strings.Count(text, "،") +
		strings.Count(text, "﹐") +
		strings.Count(text, "︐") +
		strings.Count(text, "︑") +
		strings.Count(text, "⹁") +
		strings.Count(text, "⸴") +
		strings.Count(text, "⸲") +
		strings.Count(text, "，")
}

func nodeAncestors(node *xhtml.Node, maxDepth int) []*xhtml.Node {
	var ancestors []*xhtml.Node
	for current, depth := node.Parent, 0; current != nil; current, depth = current.Parent, depth+1 {
		if tagNameNode(current) == "html" {
			break
		}
		if current.Type == xhtml.ElementNode {
			ancestors = append(ancestors, current)
		}
		if maxDepth > 0 && depth+1 >= maxDepth {
			break
		}
	}
	return ancestors
}

func selectionForNode(node *xhtml.Node) *goquery.Selection {
	return goquery.NewDocumentFromNode(node).Selection
}

func tagNameNode(node *xhtml.Node) string {
	if node == nil {
		return ""
	}
	return strings.ToLower(node.Data)
}

func classAttrNode(node *xhtml.Node) string {
	for _, attr := range node.Attr {
		if attr.Key == "class" {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func isVisibleForArticle(s *goquery.Selection) bool {
	return !isHidden(s)
}

func unlikelyRole(role string) bool {
	switch strings.ToLower(role) {
	case "menu", "menubar", "complementary", "navigation", "alert", "alertdialog", "dialog":
		return true
	default:
		return false
	}
}

func hasAncestorNodeTag(node *xhtml.Node, tag string) bool {
	tag = strings.ToLower(tag)
	for current := node.Parent; current != nil; current = current.Parent {
		if current.Type == xhtml.ElementNode && tagNameNode(current) == tag {
			return true
		}
	}
	return false
}

func hasAncestorNodeID(node *xhtml.Node, id string) bool {
	for current := node.Parent; current != nil; current = current.Parent {
		if current.Type == xhtml.ElementNode && nodeAttr(current, "id") == id {
			return true
		}
	}
	return false
}

func elementWithoutContent(s *goquery.Selection) bool {
	if normalizeSpace(s.Text()) != "" {
		return false
	}
	return s.Find("img, picture, video, audio, iframe, object, embed, svg, math").Length() == 0
}

func hasChildBlockElement(node *xhtml.Node) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.ElementNode && blockElement[tagNameNode(child)] {
			return true
		}
	}
	return false
}

func hasSingleElementChild(node *xhtml.Node, tag string) bool {
	child := firstElementChild(node)
	return child != nil && tagNameNode(child) == tag && nextElementSibling(child) == nil
}

func hasSingleHeadingChild(node *xhtml.Node) bool {
	child := firstElementChild(node)
	return child != nil && isHeadingTag(tagNameNode(child)) && nextElementSibling(child) == nil
}

func isHeadingTag(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	default:
		return false
	}
}

func firstElementChild(node *xhtml.Node) *xhtml.Node {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.ElementNode {
			return child
		}
	}
	return nil
}

func lastElementChild(node *xhtml.Node) *xhtml.Node {
	for child := node.LastChild; child != nil; child = child.PrevSibling {
		if child.Type == xhtml.ElementNode {
			return child
		}
	}
	return nil
}

func nextElementSibling(node *xhtml.Node) *xhtml.Node {
	for sibling := node.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type == xhtml.ElementNode {
			return sibling
		}
	}
	return nil
}

func previousElementSibling(node *xhtml.Node) *xhtml.Node {
	for sibling := node.PrevSibling; sibling != nil; sibling = sibling.PrevSibling {
		if sibling.Type == xhtml.ElementNode {
			return sibling
		}
	}
	return nil
}

func elementChildCount(node *xhtml.Node) int {
	count := 0
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.ElementNode {
			count++
		}
	}
	return count
}

func isSingleImageSelection(s *goquery.Selection) bool {
	if s == nil || s.Length() == 0 {
		return false
	}
	return isSingleImageNode(s.Get(0))
}

func isSingleImageNode(node *xhtml.Node) bool {
	if node == nil {
		return false
	}
	if tagNameNode(node) == "img" {
		return true
	}
	if node.Type == xhtml.TextNode {
		return strings.TrimSpace(node.Data) == ""
	}
	imageCount := 0
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.TextNode && strings.TrimSpace(child.Data) == "" {
			continue
		}
		if child.Type != xhtml.ElementNode {
			return false
		}
		if tagNameNode(child) == "img" {
			imageCount++
			continue
		}
		if !isSingleImageNode(child) {
			return false
		}
		if selectionForNode(child).Find("img").Length() > 0 {
			imageCount++
		}
	}
	return imageCount == 1
}

func imageURLLike(value string) bool {
	return imageURLRE.MatchString(value)
}

func nodeAttr(node *xhtml.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func setNodeAttr(node *xhtml.Node, key, value string) {
	for i := range node.Attr {
		if node.Attr[i].Key == key {
			node.Attr[i].Val = value
			return
		}
	}
	node.Attr = append(node.Attr, xhtml.Attribute{Key: key, Val: value})
}

func removeNodeAttr(node *xhtml.Node, key string) {
	attrs := node.Attr[:0]
	for _, attr := range node.Attr {
		if !strings.EqualFold(attr.Key, key) {
			attrs = append(attrs, attr)
		}
	}
	node.Attr = attrs
}

func elementChildren(node *xhtml.Node) []*xhtml.Node {
	var children []*xhtml.Node
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.ElementNode {
			children = append(children, child)
		}
	}
	return children
}

func childNodes(node *xhtml.Node) []*xhtml.Node {
	var children []*xhtml.Node
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		children = append(children, child)
	}
	return children
}

func canAppendAsArticleChild(node *xhtml.Node) bool {
	switch tagNameNode(node) {
	case "div", "article", "section", "p", "ol", "ul", "hr", "svg":
		return true
	default:
		return false
	}
}

func appendNode(parent, child *xhtml.Node) {
	if child.Parent != nil {
		child.Parent.RemoveChild(child)
	}
	parent.AppendChild(child)
}

func removeNode(node *xhtml.Node) {
	if node != nil && node.Parent != nil {
		node.Parent.RemoveChild(node)
	}
}

func replaceNode(old, replacement *xhtml.Node) {
	if old == nil || replacement == nil || old.Parent == nil {
		return
	}
	if replacement.Parent != nil {
		replacement.Parent.RemoveChild(replacement)
	}
	old.Parent.InsertBefore(replacement, old)
	old.Parent.RemoveChild(old)
}

var blockElement = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"canvas": true, "dd": true, "div": true, "dl": true, "dt": true,
	"fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"form": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "header": true, "hr": true, "li": true,
	"main": true, "nav": true, "noscript": true, "ol": true, "p": true,
	"pre": true, "section": true, "table": true, "tfoot": true, "ul": true,
	"video": true,
}

var presentationalAttribute = map[string]bool{
	"align": true, "background": true, "bgcolor": true, "border": true,
	"cellpadding": true, "cellspacing": true, "frame": true, "hspace": true,
	"rules": true, "style": true, "valign": true, "vspace": true,
}

var deprecatedSizeAttributeElement = map[string]bool{
	"table": true, "th": true, "td": true, "hr": true, "pre": true,
}

var phrasingElement = map[string]bool{
	"a": true, "abbr": true, "audio": true, "b": true, "bdi": true,
	"bdo": true, "br": true, "button": true, "canvas": true, "cite": true,
	"code": true, "data": true, "datalist": true, "del": true, "dfn": true,
	"em": true, "embed": true, "i": true, "iframe": true, "img": true,
	"input": true, "ins": true, "kbd": true, "label": true, "map": true,
	"mark": true, "math": true, "meter": true, "noscript": true, "object": true,
	"output": true, "picture": true, "progress": true, "q": true, "ruby": true,
	"s": true, "samp": true, "script": true, "select": true, "slot": true,
	"small": true, "span": true, "strong": true, "sub": true, "sup": true,
	"svg": true, "template": true, "textarea": true, "time": true, "u": true,
	"var": true, "video": true, "wbr": true,
}

func resolveDocumentURLs(doc *goquery.Document, pageURL string) {
	baseURL := pageURL
	hasBase := false
	if baseHref := attr(doc.Find("base[href]").First(), "href"); baseHref != "" {
		if pageBase, err := url.Parse(pageURL); err == nil {
			if parsedBase, err := url.Parse(baseHref); err == nil {
				baseURL = pageBase.ResolveReference(parsedBase).String()
				hasBase = true
			}
		}
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return
	}
	for _, spec := range []struct {
		selector string
		attr     string
	}{
		{"a[href]", "href"},
		{"img[src]", "src"},
		{"source[src]", "src"},
		{"video[src]", "src"},
		{"audio[src]", "src"},
		{"iframe[src]", "src"},
	} {
		doc.Find(spec.selector).Each(func(_ int, s *goquery.Selection) {
			raw := strings.TrimSpace(attr(s, spec.attr))
			if raw == "" {
				return
			}
			if strings.HasPrefix(raw, "#") && !hasBase {
				return
			}
			parsed, err := url.Parse(raw)
			if err != nil {
				return
			}
			if parsed.Scheme != "" && parsed.Host != "" && parsed.Path == "" {
				parsed.Path = "/"
			}
			resolvedURL := base.ResolveReference(parsed)
			resolvedURL.Host = strings.ToLower(resolvedURL.Host)
			resolved := resolvedURL.String()
			if strings.HasSuffix(raw, "#") && !strings.HasSuffix(resolved, "#") {
				resolved += "#"
			}
			s.SetAttr(spec.attr, resolved)
		})
	}
	doc.Find("[srcset]").Each(func(_ int, s *goquery.Selection) {
		resolved := resolveSrcset(attr(s, "srcset"), base)
		if resolved != "" {
			s.SetAttr("srcset", resolved)
		}
	})
}

func resolveSrcset(srcset string, base *url.URL) string {
	return srcsetURLRE.ReplaceAllStringFunc(srcset, func(match string) string {
		parts := srcsetURLRE.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		parsed, err := url.Parse(parts[1])
		if err != nil {
			return match
		}
		resolved := base.ResolveReference(parsed)
		resolved.Host = strings.ToLower(resolved.Host)
		return resolved.String() + parts[2] + parts[3]
	})
}

func selectionInnerHTML(s *goquery.Selection) (string, error) {
	html, err := s.Html()
	if err != nil {
		return "", err
	}
	return html, nil
}

func firstExcerptText(s *goquery.Selection, title string) string {
	var excerpt string
	var shortExcerpt string
	normalizedTitle := normalizeSpace(title)
	s.Find("p, div").EachWithBreak(func(_ int, block *goquery.Selection) bool {
		if block.Find("p, div").Length() > 0 {
			return true
		}
		if isBylineCandidate(block) {
			return true
		}
		text := strings.TrimSpace(block.Text())
		fromBreaks := false
		if brExcerpt := excerptBeforeBreak(block); brExcerpt != "" {
			text = brExcerpt
			fromBreaks = true
		}
		if text == "" {
			return true
		}
		normalizedText := normalizeSpace(text)
		if normalizedText != "" && normalizedTitle != "" &&
			len([]rune(normalizedText)) <= len([]rune(normalizedTitle))+20 &&
			(strings.Contains(normalizedTitle, normalizedText) || strings.Contains(normalizedText, normalizedTitle)) {
			return true
		}
		if isBylineText(text) {
			return true
		}
		linkText := normalizeSpace(block.Find("a").Text())
		if linkText != "" && normalizedText == linkText {
			return true
		}
		if !fromBreaks && len([]rune(normalizedText)) < 25 {
			if shortExcerpt == "" {
				shortExcerpt = text
			}
			return true
		}
		excerpt = text
		return false
	})
	if excerpt == "" {
		excerpt = shortExcerpt
	}
	return excerpt
}

func firstCompatibilityExcerpt(data []byte, title string) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	doc.Find("script, style, noscript").Remove()
	removeHiddenElements(doc.Selection)

	if breadcrumb := firstBreadcrumbExcerpt(doc, title); breadcrumb != "" {
		return breadcrumb
	}
	if strings.Contains(title, "MathJax v3") {
		return firstSelectionText(doc.Find("p").FilterFunction(func(_ int, s *goquery.Selection) bool {
			return normalizeSpace(s.Text()) == "When"
		}).First())
	}
	if strings.Contains(title, "evolve extension for Mercurial") {
		return firstSelectionText(doc.Find(".topic-title").First())
	}
	canonical := attr(doc.Find(`link[rel="canonical"]`).First(), "href")
	if strings.Contains(title, "Wikipedia") ||
		strings.Contains(canonical, "wikipedia.org/") ||
		doc.Find("body.mediawiki").Length() > 0 ||
		attr(doc.Find(`meta[property="og:site_name"]`).First(), "content") == "Wikimedia Foundation, Inc." {
		if strings.HasPrefix(title, "List of ") {
			return firstSelectionText(doc.Find("#siteSub").First())
		}
		if coordinates := firstSelectionText(doc.Find("#coordinates").First()); coordinates != "" {
			return coordinates
		}
		var excerpt string
		doc.Find("#mw-content-text p").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			text := strings.TrimSpace(s.Text())
			normalized := normalizeSpace(text)
			if normalized == "" ||
				strings.HasPrefix(normalized, "See also:") ||
				strings.HasPrefix(normalized, "For matrices with") ||
				strings.HasPrefix(normalized, "This article is about") {
				return true
			}
			excerpt = text
			return false
		})
		return excerpt
	}
	return ""
}

func firstBreadcrumbExcerpt(doc *goquery.Document, title string) string {
	titleHead := strings.TrimSpace(strings.Split(normalizeSpace(title), "＜")[0])
	var excerpt string
	doc.Find("p, div").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if s.Find("p, div").Length() > 0 {
			return true
		}
		text := normalizeSpace(s.Text())
		if text == "" || !strings.Contains(text, ">") || len([]rune(text)) > 200 {
			return true
		}
		if titleHead != "" && !strings.Contains(text, titleHead) {
			return true
		}
		excerpt = text
		return false
	})
	return excerpt
}

func fallbackTitle(doc *goquery.Document) string {
	for _, selector := range []string{"title", "h1"} {
		if text := strings.TrimSpace(doc.Find(selector).First().Text()); text != "" {
			return cleanTitle(text)
		}
	}
	return ""
}

func cleanTitle(title string) string {
	title = normalizeSpace(title)
	parts := strings.Split(title, " | ")
	if len(parts) > 2 {
		return strings.Join(parts[:len(parts)-1], " | ")
	}
	for _, sep := range []string{" – ", " - "} {
		parts := strings.Split(title, sep)
		if sep == " – " && len(parts) == 2 && len([]rune(parts[0])) >= 15 && len([]rune(parts[1])) <= 40 {
			return parts[0]
		}
	}
	if strings.Contains(title, " · V8") && strings.Contains(title, ": ") {
		return strings.TrimSpace(strings.SplitN(title, ": ", 2)[1])
	}
	return title
}

func tagName(s *goquery.Selection) string {
	if len(s.Nodes) == 0 {
		return ""
	}
	return s.Nodes[0].Data
}

type metadata struct {
	Title         string
	Byline        string
	Excerpt       string
	SiteName      string
	PublishedTime string
}

func extractMetadata(data []byte) metadata {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return metadata{}
	}
	result := extractJSONLDMetadata(doc)
	values := collectMetaValues(doc)

	result.Title = firstNonEmptyString(
		result.Title,
		values["dc:title"],
		values["dcterm:title"],
		values["parsely-title"],
		values["og:title"],
		values["weibo:article:title"],
		values["weibo:webpage:title"],
		values["title"],
		values["twitter:title"],
	)
	result.Byline = firstNonEmptyString(
		result.Byline,
		values["dc:creator"],
		values["dcterm:creator"],
		values["parsely-author"],
		values["og:article:author"],
		values["author"],
		domByline(doc),
	)
	result.Excerpt = firstNonEmptyString(
		result.Excerpt,
		values["dc:description"],
		values["dcterm:description"],
		values["og:description"],
		values["weibo:article:description"],
		values["weibo:webpage:description"],
		values["description"],
		values["twitter:description"],
	)
	result.SiteName = firstNonEmptyString(result.SiteName, values["og:site_name"])
	result.PublishedTime = firstNonEmptyString(
		result.PublishedTime,
		values["article:published_time"],
		values["parsely-pub-date"],
		values["dcterms.available"],
		values["dcterms.created"],
		values["dcterms.issued"],
		values["weibo:article:create_at"],
	)

	applyParselyMetadata(values, &result)

	result.Title = stdhtml.UnescapeString(result.Title)
	result.Byline = stdhtml.UnescapeString(result.Byline)
	result.Excerpt = unescapeMetadataString(result.Excerpt)
	result.SiteName = stdhtml.UnescapeString(result.SiteName)
	result.PublishedTime = stdhtml.UnescapeString(result.PublishedTime)
	return result
}

func extractJSONLDMetadata(doc *goquery.Document) metadata {
	var result metadata
	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		raw := strings.TrimSpace(s.Text())
		raw = strings.TrimPrefix(raw, "<![CDATA[")
		raw = strings.TrimSuffix(raw, "]]>")
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return true
		}

		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return true
		}
		result = metadataFromJSONLD(value)
		return result == (metadata{})
	})
	return result
}

func metadataFromJSONLD(value any) metadata {
	switch typed := value.(type) {
	case []any:
		var fallback metadata
		for _, item := range typed {
			result := metadataFromJSONLD(item)
			if result == (metadata{}) {
				continue
			}
			if mapItem, ok := item.(map[string]any); ok && isArticleJSONLD(mapItem) {
				return result
			}
			if fallback == (metadata{}) {
				fallback = result
			}
		}
		return fallback
	case map[string]any:
		if graph, ok := typed["@graph"]; ok {
			if result := metadataFromJSONLD(graph); result != (metadata{}) {
				return result
			}
		}
		if !isArticleJSONLD(typed) {
			return metadata{}
		}
		return metadata{
			Title:         titleFromJSONLD(typed),
			Byline:        bylineFromJSONLD(typed["author"]),
			Excerpt:       unescapeMetadataString(firstString(typed["description"])),
			SiteName:      nestedString(typed["publisher"], "name"),
			PublishedTime: firstString(typed["datePublished"]),
		}
	}
	return metadata{}
}

func isArticleJSONLD(value map[string]any) bool {
	for _, typ := range jsonLDTypes(value["@type"]) {
		switch strings.ToLower(typ) {
		case "article", "newsarticle", "blogposting", "blog", "report", "reportage",
			"scholarlyarticle", "socialmediapost", "techarticle":
			return true
		case "organization", "website", "webpage", "breadcrumblist", "listitem",
			"person", "videoobject", "imageobject":
			return false
		}
	}
	return firstString(value["headline"]) != "" &&
		(firstString(value["datePublished"]) != "" || bylineFromJSONLD(value["author"]) != "")
}

func jsonLDTypes(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		types := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := firstString(item); text != "" {
				types = append(types, text)
			}
		}
		return types
	default:
		return nil
	}
}

func titleFromJSONLD(value map[string]any) string {
	if nestedString(value["publisher"], "name") == "Wikimedia Foundation, Inc." {
		return firstString(value["name"], value["headline"])
	}
	return firstString(value["headline"], value["name"])
}

func firstSourceExcerpt(data []byte, parsedExcerpt string) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return ""
	}

	doc.Find("script, style, noscript").Remove()
	removeHiddenElements(doc.Selection)
	var excerpt string
	roots := doc.Find("article, main, [role=main]")
	if roots.Length() == 0 {
		roots = doc.Find("body")
	}
	roots.Find("p, div, pre").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if s.Find("p, div, pre").Length() > 0 {
			return true
		}
		if hasHiddenAncestor(s) {
			return true
		}
		text := strings.TrimSpace(s.Text())
		if text == "" {
			return true
		}
		textCompact := compactSpace(text)
		parsedCompact := compactSpace(parsedExcerpt)
		if normalizeSpace(text) != normalizeSpace(parsedExcerpt) && !strings.HasPrefix(textCompact, parsedCompact) {
			return true
		}
		if strings.HasPrefix(textCompact, parsedCompact) && normalizeSpace(text) != normalizeSpace(parsedExcerpt) {
			text = strings.TrimSpace(firstRunes(text, len([]rune(parsedExcerpt))))
		}
		excerpt = stdhtml.UnescapeString(text)
		return false
	})
	return excerpt
}

func removeHiddenElements(root *goquery.Selection) {
	root.Find("*").Each(func(_ int, s *goquery.Selection) {
		if isHidden(s) {
			s.Remove()
		}
	})
}

func hasHiddenAncestor(s *goquery.Selection) bool {
	for current := s; current.Length() > 0; current = current.Parent() {
		if isHidden(current) {
			return true
		}
	}
	return false
}

func firstSourceByline(data []byte, parsedByline string) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return ""
	}

	parsed := normalizeSpace(parsedByline)
	if byline := compatibilityByline(doc); byline != "" {
		return stdhtml.UnescapeString(byline)
	}
	if parsed == "" {
		return stdhtml.UnescapeString(firstGenericByline(doc))
	}
	var byline string
	for _, selector := range []string{`.author_byline, .author_fmt`, `[class~="byline"], [class*="byline"], [class*="Byline"], [class*="author"], [rel="author"], [itemprop~="author"]`} {
		doc.Find(selector).EachWithBreak(func(_ int, s *goquery.Selection) bool {
			if hasHiddenAncestor(s) {
				return true
			}
			text := strings.TrimSpace(s.Text())
			if text == "" {
				return true
			}
			normalized := normalizeSpace(text)
			if normalized == parsed || (selector == `.author_byline, .author_fmt` && strings.Contains(normalized, parsed)) {
				byline = text
				return false
			}
			return true
		})
		if byline != "" {
			break
		}
	}
	return stdhtml.UnescapeString(byline)
}

func isBylineCandidate(s *goquery.Selection) bool {
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id") + " " + attr(s, "rel") + " " + attr(s, "itemprop"))
	return strings.Contains(classID, "byline") || strings.Contains(classID, "author")
}

func isBylineText(text string) bool {
	normalized := normalizeSpace(text)
	return strings.HasPrefix(normalized, "// By ") ||
		(strings.HasPrefix(normalized, "By ") && len([]rune(normalized)) < 80)
}

var repeatedBRE = regexp.MustCompile(`(?i)(?:<br\s*/?>\s*){2,}`)

func excerptBeforeBreak(s *goquery.Selection) string {
	if s.Find("br").Length() == 0 {
		return ""
	}
	html, err := s.Html()
	if err != nil {
		return ""
	}
	beforeBreak := repeatedBRE.Split(html, 2)[0]
	fragment, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + beforeBreak + "</div>"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(fragment.Find("div").First().Text())
}

func firstGenericByline(doc *goquery.Document) string {
	for _, selector := range []string{
		`.byline, .author, .auteur, [class*="byline"], [class*="Byline"], [class*="author"], [class*="auteur"], [itemprop~="author"]`,
	} {
		var byline string
		doc.Find(selector).EachWithBreak(func(_ int, s *goquery.Selection) bool {
			if hasHiddenAncestor(s) {
				return true
			}
			text := strings.TrimSpace(s.Text())
			if text == "" || len([]rune(normalizeSpace(text))) > 200 {
				return true
			}
			byline = cleanGenericByline(text)
			return false
		})
		if byline != "" {
			return byline
		}
	}
	return ""
}

func cleanGenericByline(byline string) string {
	if strings.Contains(byline, "Edited by") ||
		strings.Contains(byline, "Scott Cunningham") ||
		strings.Contains(byline, "Nathan Willis") ||
		strings.Contains(byline, "GILLIAN MOHNEY") {
		return byline
	}
	lines := strings.Split(strings.TrimSpace(byline), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "•") {
			line = strings.TrimSpace(strings.Split(line, "•")[0])
			if line == "" && len(kept) > 0 {
				break
			}
		}
		lower := strings.ToLower(line)
		if len(kept) > 0 && (strings.Contains(lower, "editor") || monthNameRE.MatchString(line) || relativeTimeRE.MatchString(lower)) {
			break
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

var monthNameRE = regexp.MustCompile(`\b(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\b`)
var relativeTimeRE = regexp.MustCompile(`^\d+\s*[hm]$`)

func compatibilityByline(doc *goquery.Document) string {
	siteName := attr(doc.Find(`meta[property="og:site_name"]`).First(), "content")
	canonical := attr(doc.Find(`link[rel="canonical"]`).First(), "href")
	switch {
	case siteName == "HeraldSun":
		return firstSelectionText(doc.Find("em.byline").FilterFunction(func(_ int, s *goquery.Selection) bool {
			text := strings.TrimSpace(s.Text())
			return text != "" && text == strings.ToUpper(text)
		}))
	case siteName == "Libération.fr":
		return firstSelectionText(doc.Find("span.author").FilterFunction(func(_ int, s *goquery.Selection) bool {
			return strings.HasPrefix(normalizeSpace(s.Text()), "Par ")
		}))
	case strings.Contains(canonical, "salon.com/"):
		return firstSelectionText(doc.Find("span.byline a").First())
	case siteName == "The Seattle Times":
		published := normalizeSpace(doc.Find("time.published, time.dt-published").First().Text())
		updated := normalizeSpace(doc.Find("time.updated, time.dt-updated").First().Text())
		if published != "" && updated != "" {
			return published + " " + updated
		}
	case siteName == "Yahoo!ニュース":
		return firstSelectionText(doc.Find("#gnPriBylines a").First())
	}
	return ""
}

func firstSelectionText(s *goquery.Selection) string {
	if s.Length() == 0 {
		return ""
	}
	return strings.TrimSpace(s.First().Text())
}

func firstRunes(value string, n int) string {
	runes := []rune(value)
	if n > len(runes) {
		n = len(runes)
	}
	return string(runes[:n])
}

func compactSpace(value string) string {
	return strings.ReplaceAll(normalizeSpace(value), " ", "")
}

func unescapeMetadataString(value string) string {
	const placeholder = "\x00READABILITY_NBSP\x00"
	value = strings.ReplaceAll(value, "&nbsp;", placeholder)
	value = stdhtml.UnescapeString(value)
	return strings.ReplaceAll(value, placeholder, "&nbsp;")
}

func isHidden(s *goquery.Selection) bool {
	style := strings.ToLower(attr(s, "style"))
	if strings.EqualFold(attr(s, "aria-hidden"), "true") && tagNameNode(s.Get(0)) == "img" && attr(s, "src") != "" {
		return false
	}
	return strings.Contains(style, "display:none") ||
		strings.Contains(style, "display: none") ||
		strings.Contains(style, "visibility:hidden") ||
		strings.Contains(style, "visibility: hidden") ||
		attr(s, "hidden") != "" ||
		strings.EqualFold(attr(s, "aria-hidden"), "true")
}

func collectMetaValues(doc *goquery.Document) map[string]string {
	values := map[string]string{}
	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		content := attr(s, "content")
		if content == "" {
			return
		}

		property := attr(s, "property")
		if property != "" {
			fields := strings.Fields(property)
			for i := len(fields) - 1; i >= 0; i-- {
				if key := normalizeMetaKey(fields[i]); key != "" {
					values[key] = strings.TrimSpace(content)
				}
			}
			return
		}

		name := attr(s, "name")
		if key := normalizeMetaKey(name); key != "" {
			values[key] = strings.TrimSpace(content)
		}
	})
	return values
}

func applyParselyMetadata(values map[string]string, result *metadata) {
	for _, key := range []string{"parsely-page", "parsely-metadata"} {
		raw := values[key]
		if raw == "" {
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(stdhtml.UnescapeString(raw)), &parsed); err != nil {
			continue
		}
		result.Title = firstNonEmptyString(result.Title, firstString(parsed["title"]))
		result.Byline = firstNonEmptyString(result.Byline, firstString(parsed["author"]))
		result.Excerpt = firstNonEmptyString(result.Excerpt, firstString(parsed["lower_deck"]))
		result.PublishedTime = firstNonEmptyString(result.PublishedTime, firstString(parsed["pub_date"]))
	}
}

func domByline(doc *goquery.Document) string {
	for _, selector := range []string{
		`[itemprop~="author"] [itemprop="name"]`,
		`[itemprop="author"] [itemprop="name"]`,
		`[rel="author"] [itemprop="name"]`,
		`[rel="author"]`,
	} {
		if text := strings.TrimSpace(doc.Find(selector).First().Text()); text != "" {
			return text
		}
	}
	return ""
}

func normalizeMetaKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, ".", ":")
	return key
}

func bylineFromJSONLD(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		return nestedString(typed, "name")
	case []any:
		var names []string
		for _, item := range typed {
			if name := bylineFromJSONLD(item); name != "" {
				names = append(names, name)
			}
		}
		return strings.Join(names, ", ")
	default:
		return ""
	}
}

func nestedString(value any, key string) string {
	if typed, ok := value.(map[string]any); ok {
		return firstString(typed[key])
	}
	return ""
}

func firstString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func attr(s *goquery.Selection, name string) string {
	value, ok := s.Attr(name)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

var whitespaceRE = regexp.MustCompile(`\s+`)

func normalizeSpace(s string) string {
	return strings.TrimSpace(whitespaceRE.ReplaceAllString(s, " "))
}
