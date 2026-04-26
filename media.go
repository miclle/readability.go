package readability

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// removeUnusedSVGSymbols drops the share-icon symbol from inline SVG
// sprites that look like media-player controls. We key off the presence
// of play/pause/fullscreen/video symbols (a strong signal the SVG is a
// player skin) and remove the share symbol because it consistently
// appears as a UI affordance the cleanup pass would otherwise miss —
// readers don't need a share icon embedded in the rendered article.
func removeUnusedSVGSymbols(root *goquery.Selection) {
	root.Find("svg").Each(func(_ int, s *goquery.Selection) {
		if s.Find("symbol#play, symbol#pause, symbol#fullscreen, symbol#video").Length() == 0 {
			return
		}
		s.Find("symbol#share").Remove()
	})
}

// removeEmptyMediaHeadings deletes h1/h2 elements that contain only an
// SVG decoration (no text). These typically come from CMS templates that
// use icon-only headings for navigational accents — they survive scoring
// because the SVG counts as content, but they add no information to the
// rendered article.
func removeEmptyMediaHeadings(root *goquery.Selection) {
	root.Find("h1, h2").Each(func(_ int, s *goquery.Selection) {
		if normalizeSpace(s.Text()) != "" || s.Find("svg").Length() == 0 {
			return
		}
		s.Remove()
	})
}

// fixLazyImages mirrors mozilla/readability's `_fixLazyImages`: many
// sites emit a tiny base64 placeholder as the visible `src` and stash
// the real image URL in a data-* attribute (data-src, data-original,
// data-lazy-srcset, ...). Two passes:
//  1. If the visible src is a base64 data URL AND another attribute on
//     the same element looks like an image URL AND the data URL is small
//     (< 133 chars beyond the prefix, matching upstream's heuristic),
//     drop the placeholder src so step 2 is allowed to run. SVG data
//     URLs are preserved because they may be the real image.
//  2. For any element that has no usable src/srcset (or has a "lazy"
//     class), copy the first URL-shaped data-* attribute into src or
//     srcset (depending on whether it looks like a single URL or a
//     comma-separated set). For <figure> we synthesize a child <img>
//     when none exists yet, so the lazy-loaded image still scores.
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

// preservableMediaCount counts embedded media inside `s` that
// conditional cleanup must keep — i.e. legitimate audio/video/iframe
// content. Three categories qualify:
//   - <audio src=…>: a real audio element with a source URL.
//   - <video> with a blob: src or data-video-id attribute, which signals
//     a player binding to a real stream even if the src looks empty.
//   - any iframe/object/embed/audio/video whose attributes contain a URL
//     matched by cfg.videoAllowed (the parser-configurable allow list).
//
// The result feeds conditional cleanup's "this looks like a media-rich
// article" exemption so player blocks aren't stripped along with
// surrounding boilerplate.
func preservableMediaCount(s *goquery.Selection, cfg parserConfig) int {
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
			if cfg.videoAllowed(attr.Val) {
				count++
				return
			}
		}
	})
	return count
}

// removableEmbedCount is the inverse of preservableMediaCount: it
// counts object/embed/iframe elements that are NOT on the video allow
// list (neither attribute values nor inner HTML match cfg.videoAllowed).
// Conditional cleanup uses this number as part of the "embed weight" in
// the link-density formula: lots of unrecognized embeds push the
// element toward removal, even when its text-to-link ratio looks fine.
func removableEmbedCount(s *goquery.Selection, cfg parserConfig) int {
	count := 0
	s.Find("object, embed, iframe").Each(func(_ int, embed *goquery.Selection) {
		allowed := false
		for _, attr := range embed.Get(0).Attr {
			if cfg.videoAllowed(attr.Val) {
				allowed = true
				break
			}
		}
		if !allowed {
			if html, err := selectionInnerHTML(embed); err == nil && cfg.videoAllowed(html) {
				allowed = true
			}
		}
		if !allowed {
			count++
		}
	})
	return count
}
