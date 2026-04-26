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
			}
			assertMetadataMatches(t, article, expected)
		})
	}
	t.Logf("Mozilla metadata fixtures: %d passing, %d failing", len(names)-failed, failed)
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
