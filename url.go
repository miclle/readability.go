package readability

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

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
			if spec.attr == "src" && strings.HasPrefix(raw, "//") && tagNameNode(s.Get(0)) == "iframe" {
				return
			}
			parsed, err := url.Parse(raw)
			if err != nil {
				if unescaped, unescapeErr := url.PathUnescape(raw); unescapeErr == nil {
					raw = unescaped
				}
				parsed = &url.URL{Path: raw}
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
