package readability

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// nodeAncestors walks up the DOM from `node` collecting element ancestors.
// The walk stops at <html> (excluded) so callers don't see the document
// root in the result. When maxDepth > 0 the slice is capped at that many
// levels, which the scoring pass uses to bound how far a paragraph's
// content score can propagate up the tree.
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

// selectionForNode wraps a single xhtml.Node in a goquery.Selection so
// helpers that already accept *goquery.Selection can also operate on raw
// nodes. The new Document is intentionally cheap (no parser pass) since
// the node is already attached to its real tree; callers should not rely
// on the wrapper's document identity.
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

func hasAncestorNodeAttrMatching(node *xhtml.Node, key string, match func(string) bool) bool {
	for current := node.Parent; current != nil; current = current.Parent {
		if current.Type != xhtml.ElementNode {
			continue
		}
		if match(nodeAttr(current, key)) {
			return true
		}
	}
	return false
}

func hasAncestorNodeMatching(node *xhtml.Node, match func(*xhtml.Node) bool) bool {
	for current := node.Parent; current != nil; current = current.Parent {
		if current.Type == xhtml.ElementNode && match(current) {
			return true
		}
	}
	return false
}

// elementWithoutContent returns true when the element has no text and
// contains no embedded media. Scoring uses this to drop empty wrapper
// divs/sections that survived initial cleanup; we treat
// img/picture/video/audio/iframe/object/embed/svg/math as content even
// when they carry no text, because those elements ARE the article in
// gallery / video / equation pages.
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

// isSingleImageNode reports whether the subtree rooted at `node` is
// effectively a single image: the node itself is <img>, or it is a chain
// of element wrappers (figure > picture > img, figure > a > img, ...)
// containing exactly one <img> with only whitespace text around it. The
// noscript-image unwrap path uses this to detect placeholder wrappers
// before swapping in the noscript-supplied real image.
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

// canAppendAsArticleChild reports whether `node` is a tag we let the
// final article wrapper hold directly. Anything else (li, td, dd, ...)
// is rewritten to <div> by buildArticleContent so the rendered output
// stays valid HTML even when a list-style sibling scored high enough to
// be included as part of the article body.
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
	"main": true, "nav": true, "noscript": true, "ol": true, "p": true, "picture": true,
	"pre": true, "section": true, "table": true, "tbody": true, "td": true, "tfoot": true,
	"th": true, "thead": true, "tr": true, "ul": true,
	"svg": true, "video": true,
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

func selectionInnerHTML(s *goquery.Selection) (string, error) {
	return selectionInnerHTMLWithNBSP(s, "&nbsp;")
}

func selectionInnerHTMLWithNBSP(s *goquery.Selection, nbspEntity string) (string, error) {
	html, err := s.Html()
	if err != nil {
		return "", err
	}
	return normalizeSerializedTextEntities(html, nbspEntity), nil
}

// normalizeSerializedTextEntities is a single-pass HTML serializer
// patcher: in attribute regions (between `<` and `>`) it normalizes
// numeric quote entities to their named forms (&apos; / &quot;) so the
// output matches mozilla/readability's serialization; in text regions it
// unescapes the same numeric entities to their raw quote characters and
// rewrites U+00A0 to the configured nbsp entity. The state machine is
// intentionally tag-shape-aware (rather than full HTML aware) because
// goquery's serializer already produces well-formed output, so a string
// scan is enough to recover the upstream-compatible rendering.
func normalizeSerializedTextEntities(value, nbspEntity string) string {
	var out strings.Builder
	out.Grow(len(value))
	inTag := false
	start := 0
	for i, r := range value {
		switch r {
		case '<':
			if !inTag {
				out.WriteString(unescapeQuoteEntities(value[start:i], nbspEntity))
				start = i
				inTag = true
			}
		case '>':
			if inTag {
				out.WriteString(normalizeAttributeQuoteEntities(value[start : i+1]))
				start = i + 1
				inTag = false
			}
		}
	}
	if start < len(value) {
		rest := value[start:]
		if inTag {
			out.WriteString(normalizeAttributeQuoteEntities(rest))
		} else {
			out.WriteString(unescapeQuoteEntities(rest, nbspEntity))
		}
	}
	return out.String()
}

func normalizeAttributeQuoteEntities(value string) string {
	value = strings.ReplaceAll(value, "&#39;", "&apos;")
	value = strings.ReplaceAll(value, "&#34;", "&quot;")
	return value
}

func unescapeQuoteEntities(value, nbspEntity string) string {
	value = strings.ReplaceAll(value, "&#39;", "'")
	value = strings.ReplaceAll(value, "&#34;", `"`)
	value = strings.ReplaceAll(value, "\u00a0", nbspEntity)
	return value
}
