package readability

import (
	"io"
	"math"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ReaderableOptions controls the fast readerability check.
type ReaderableOptions struct {
	MinContentLength int
	MinScore         int
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
	doc.Find("p, pre, article").EachWithBreak(func(_ int, s *goquery.Selection) bool {
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

func isProbablyVisible(s *goquery.Selection) bool {
	style := strings.ToLower(attr(s, "style"))
	return !strings.Contains(style, "display:none") &&
		!strings.Contains(style, "display: none") &&
		attr(s, "hidden") == "" &&
		strings.ToLower(attr(s, "aria-hidden")) != "true"
}
