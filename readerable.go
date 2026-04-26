package readability

import (
	"io"
	"math"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ReaderableOptions controls the fast readerability check.
type ReaderableOptions struct {
	// MinContentLength is the minimum normalized text length of a single
	// candidate node required to count toward readability. Defaults to 140.
	MinContentLength int

	// MinScore is the cumulative score threshold across qualifying nodes
	// required to mark the document as readerable. Defaults to 20.
	MinScore int
}

// IsProbablyReaderable reports whether an HTML stream is likely to contain an
// article suitable for parsing.
func IsProbablyReaderable(r io.Reader, options ...ReaderableOptions) (bool, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return false, err
	}

	minContentLength := 140
	minScore := 20
	if len(options) > 0 {
		if options[0].MinContentLength > 0 {
			minContentLength = options[0].MinContentLength
		}
		if options[0].MinScore > 0 {
			minScore = options[0].MinScore
		}
	}

	score := 0.0
	readerable := false
	nodes := readerableCandidates(doc)
	nodes.EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if !isProbablyVisible(s) {
			return true
		}
		matchString := attr(s, "class") + " " + attr(s, "id")
		if unlikelyCandidateRE.MatchString(matchString) && !okMaybeCandidateRE.MatchString(matchString) {
			return true
		}
		if goquery.NodeName(s.Parent()) == "li" && goquery.NodeName(s) == "p" {
			return true
		}
		textLen := len([]rune(normalizeSpace(s.Text())))
		if textLen < minContentLength {
			return true
		}
		score += math.Sqrt(float64(textLen - minContentLength))
		if score > float64(minScore) {
			readerable = true
			return false
		}
		return true
	})

	return readerable, nil
}

func readerableCandidates(doc *goquery.Document) *goquery.Selection {
	nodes := doc.Find("p, pre, article")
	doc.Find("div > br").Each(func(_ int, br *goquery.Selection) {
		parent := br.Parent()
		if parent.Length() == 0 {
			return
		}
		alreadyIncluded := false
		nodes.EachWithBreak(func(_ int, candidate *goquery.Selection) bool {
			if candidate.Get(0) == parent.Get(0) {
				alreadyIncluded = true
				return false
			}
			return true
		})
		if !alreadyIncluded {
			nodes = nodes.AddSelection(parent)
		}
	})
	return nodes
}

func isProbablyVisible(s *goquery.Selection) bool {
	style := strings.ToLower(attr(s, "style"))
	_, hidden := s.Attr("hidden")
	return !strings.Contains(style, "display:none") &&
		!strings.Contains(style, "display: none") &&
		!hidden &&
		(strings.ToLower(attr(s, "aria-hidden")) != "true" || hasFallbackImageClass(s))
}
