package readability

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

func removeUnusedSVGSymbols(root *goquery.Selection) {
	root.Find("svg").Each(func(_ int, s *goquery.Selection) {
		if s.Find("symbol#play, symbol#pause, symbol#fullscreen, symbol#video").Length() == 0 {
			return
		}
		s.Find("symbol#share").Remove()
	})
}

func removeEmptyMediaHeadings(root *goquery.Selection) {
	root.Find("h1, h2").Each(func(_ int, s *goquery.Selection) {
		if normalizeSpace(s.Text()) != "" || s.Find("svg").Length() == 0 {
			return
		}
		s.Remove()
	})
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

func preservableMediaCount(s *goquery.Selection) int {
	count := 0
	s.Find("iframe, video, audio, object, embed").Each(func(_ int, media *goquery.Selection) {
		node := media.Get(0)
		if node == nil {
			return
		}
		tag := tagNameNode(node)
		if tag == "audio" && attr(media, "src") != "" {
			count++
			return
		}
		if tag == "video" && (strings.HasPrefix(attr(media, "src"), "blob:") || attr(media, "data-video-id") != "") {
			count++
			return
		}
		for _, attr := range node.Attr {
			if videoURLRE.MatchString(attr.Val) {
				count++
				return
			}
		}
	})
	return count
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
