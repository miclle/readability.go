package readability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

type expectedMetadata struct {
	Title         string  `json:"title"`
	Byline        *string `json:"byline"`
	Excerpt       string  `json:"excerpt"`
	SiteName      *string `json:"siteName"`
	Lang          *string `json:"lang"`
	PublishedTime *string `json:"publishedTime"`
}

func TestParseMozillaMetadataFixtures(t *testing.T) {
	for _, fixture := range []string{
		"003-metadata-preferred",
		"004-metadata-space-separated-properties",
		"article-author-tag",
	} {
		t.Run(fixture, func(t *testing.T) {
			article := parseFixtureForTest(t, fixture)
			expected := readExpectedMetadataForTest(t, fixture)

			assertMetadataMatches(t, article, expected)
		})
	}
}

func TestParseAllMozillaMetadataFixtures(t *testing.T) {
	if os.Getenv("READABILITY_FULL_COMPAT") != "1" {
		t.Skip("set READABILITY_FULL_COMPAT=1 to run all Mozilla metadata fixtures")
	}

	root := filepath.Join("testdata", "test-pages")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	failed := 0
	for _, fixture := range names {
		t.Run(fixture, func(t *testing.T) {
			article := parseFixtureForTest(t, fixture)
			expected := readExpectedMetadataForTest(t, fixture)

			if !metadataMatches(article, expected) {
				failed++
				t.Logf("metadata mismatch:\ngot  %s\nwant %s", metadataForLog(articleMetadataForLog(article)), metadataForLog(expected))
			}
		})
	}
	t.Logf("Mozilla metadata fixtures: %d passing, %d failing", len(names)-failed, failed)
}

func TestCleanTitleRemovesGenericSiteSuffixAndSectionPrefix(t *testing.T) {
	got := cleanTitle("Chrome Developers: WebGPU ships · Example Blog")
	if got != "WebGPU ships" {
		t.Fatalf("cleanTitle = %q", got)
	}
}

func TestFallbackTitleUsesSingleH1WhenTitleIsTooShort(t *testing.T) {
	doc := mustTestDocument(t, `<html><head><title>Home</title></head><body><h1>This is a useful article title</h1></body></html>`)

	got := fallbackTitle(doc)
	if got != "This is a useful article title" {
		t.Fatalf("fallbackTitle = %q", got)
	}
}

func TestFallbackTitleRemovesSiteSuffix(t *testing.T) {
	doc := mustTestDocument(t, `<html><head><title>This is a useful article title - Example Site</title></head></html>`)

	got := fallbackTitle(doc)
	if got != "This is a useful article title" {
		t.Fatalf("fallbackTitle = %q", got)
	}
}

func TestFallbackTitleKeepsShortNonHierarchicalTitles(t *testing.T) {
	doc := mustTestDocument(t, `<html><head><title>Site - Useful Article Title</title></head></html>`)

	got := fallbackTitle(doc)
	if got != "Site - Useful Article Title" {
		t.Fatalf("fallbackTitle = %q", got)
	}
}

func TestFallbackTitleExtractsColonTitleWithoutMatchingHeading(t *testing.T) {
	doc := mustTestDocument(t, `<html><head><title>Example Site: This is a useful article title</title></head><body><h1>Different heading</h1></body></html>`)

	got := fallbackTitle(doc)
	if got != "This is a useful article title" {
		t.Fatalf("fallbackTitle = %q", got)
	}
}

func TestFallbackTitleKeepsColonTitleWithMatchingHeading(t *testing.T) {
	doc := mustTestDocument(t, `<html><head><title>Example Site: This is a useful article title</title></head><body><h1>Example Site: This is a useful article title</h1></body></html>`)

	got := fallbackTitle(doc)
	if got != "Example Site: This is a useful article title" {
		t.Fatalf("fallbackTitle = %q", got)
	}
}

func TestParselyTitleDoesNotOverrideOpenGraphTitle(t *testing.T) {
	result := extractMetadata([]byte(`<html><head>
		<meta property="og:title" content="Open Graph Title">
		<meta name="parsely-title" content="Parsely Title">
	</head></html>`))

	if result.Title != "Open Graph Title" {
		t.Fatalf("Title = %q", result.Title)
	}
}

func TestArticleAuthorURLIsIgnored(t *testing.T) {
	result := extractMetadata([]byte(`<html><head>
		<meta property="article:author" content="https://example.com/authors/jane">
	</head></html>`))

	if result.Byline != "" {
		t.Fatalf("Byline = %q", result.Byline)
	}
}

func TestArticleAuthorURLWithSpacedPropertyIsIgnored(t *testing.T) {
	result := extractMetadata([]byte(`<html><head>
		<meta property="article : author" content="https://example.com/authors/jane">
	</head></html>`))

	if result.Byline != "" {
		t.Fatalf("Byline = %q", result.Byline)
	}
}

func TestArticleAuthorNameIsUsed(t *testing.T) {
	result := extractMetadata([]byte(`<html><head>
		<meta property="article:author" content="Jane Doe">
	</head></html>`))

	if result.Byline != "Jane Doe" {
		t.Fatalf("Byline = %q", result.Byline)
	}
}

func TestDCTermsDatesDoNotSetPublishedTime(t *testing.T) {
	result := extractMetadata([]byte(`<html><head>
		<meta name="dcterms.created" content="2015-04-16T20:02:01Z">
		<meta name="dcterms.issued" content="2015-04-16T20:02:01Z">
	</head></html>`))

	if result.PublishedTime != "" {
		t.Fatalf("PublishedTime = %q", result.PublishedTime)
	}
}

func TestParselyPagePubDateDoesNotSetPublishedTime(t *testing.T) {
	result := extractMetadata([]byte(`<html><head>
		<meta name="parsely-page" content="{&quot;pub_date&quot;:&quot;2015-04-16T20:02:01Z&quot;}">
	</head></html>`))

	if result.PublishedTime != "" {
		t.Fatalf("PublishedTime = %q", result.PublishedTime)
	}
}

func TestSpacedMetadataPropertyIsParsed(t *testing.T) {
	result := extractMetadata([]byte(`<html><head>
		<meta property="dc : title" content="Spaced Title">
		<meta name="title" content="Fallback Title">
	</head></html>`))

	if result.Title != "Spaced Title" {
		t.Fatalf("Title = %q", result.Title)
	}
}

func parseFixtureForTest(t *testing.T, fixture string) Article {
	t.Helper()
	sourcePath := filepath.Join("testdata", "test-pages", fixture, "source.html")
	f, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	article, err := FromReader(f, "http://fakehost/test/", nil)
	if err != nil {
		t.Fatal(err)
	}
	return article
}

func readExpectedMetadataForTest(t *testing.T, fixture string) expectedMetadata {
	t.Helper()
	metadataPath := filepath.Join("testdata", "test-pages", fixture, "expected-metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var expected expectedMetadata
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	return expected
}

func assertMetadataMatches(t *testing.T, article Article, expected expectedMetadata) {
	t.Helper()
	if !metadataMatches(article, expected) {
		t.Fatalf("metadata mismatch:\ngot  %s\nwant %s", metadataForLog(articleMetadataForLog(article)), metadataForLog(expected))
	}
}

func metadataMatches(article Article, expected expectedMetadata) bool {
	if article.Title != expected.Title {
		return false
	}
	if article.Excerpt != expected.Excerpt {
		return false
	}
	if expected.Byline != nil && article.Byline != *expected.Byline {
		return false
	}
	if expected.SiteName != nil && article.SiteName != *expected.SiteName {
		return false
	}
	if expected.Lang != nil && article.Lang != *expected.Lang {
		return false
	}
	if expected.PublishedTime != nil && article.PublishedTime != *expected.PublishedTime {
		return false
	}
	return true
}

func articleMetadataForLog(article Article) expectedMetadata {
	byline := article.Byline
	siteName := article.SiteName
	lang := article.Lang
	publishedTime := article.PublishedTime
	return expectedMetadata{
		Title:         article.Title,
		Byline:        stringPtrOrNil(byline),
		Excerpt:       article.Excerpt,
		SiteName:      stringPtrOrNil(siteName),
		Lang:          stringPtrOrNil(lang),
		PublishedTime: stringPtrOrNil(publishedTime),
	}
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func metadataForLog(value expectedMetadata) string {
	data, err := json.Marshal(value)
	if err != nil {
		return err.Error()
	}
	return string(data)
}
