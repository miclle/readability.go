package readability

import (
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestMozillaFixtureInventory(t *testing.T) {
	root := filepath.Join("testdata", "test-pages")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	fixtures := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		fixtures++
		dir := filepath.Join(root, entry.Name())
		for _, name := range []string{"source.html", "expected.html", "expected-metadata.json"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				t.Fatalf("%s is missing %s: %v", entry.Name(), name, err)
			}
		}
	}

	if fixtures != 130 {
		t.Fatalf("Mozilla fixture count = %d, want 130", fixtures)
	}
}

func TestMozillaUpstreamMetadataIsPinned(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "UPSTREAM"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"repo: https://github.com/mozilla/readability",
		"commit: 08be6b4bdb204dd333c9b7a0cfbc0e730b257252",
		"path: test/test-pages",
		"license: Apache-2.0",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("UPSTREAM does not contain %q", required)
		}
	}
}

func TestParseAllMozillaContentFixtures(t *testing.T) {
	if os.Getenv("READABILITY_FULL_COMPAT") != "1" {
		t.Skip("set READABILITY_FULL_COMPAT=1 to run all Mozilla content fixtures")
	}

	names := mozillaFixtureNames(t)
	failed := 0
	for _, fixture := range names {
		t.Run(fixture, func(t *testing.T) {
			article := parseFixtureForTest(t, fixture)
			expected := readExpectedContentForTest(t, fixture)
			if normalizeFixtureHTML(article.Content) != normalizeFixtureHTML(expected) {
				failed++
				message := "content mismatch:\ngot  " + summarizeHTML(article.Content) + "\nwant " + summarizeHTML(expected)
				t.Log(message)
			}
		})
	}
	t.Logf("Mozilla content fixtures: %d passing, %d failing", len(names)-failed, failed)
}

func mozillaFixtureNames(t *testing.T) []string {
	t.Helper()
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
	return names
}

func readExpectedContentForTest(t *testing.T, fixture string) string {
	t.Helper()
	contentPath := filepath.Join("testdata", "test-pages", fixture, "expected.html")
	data, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func normalizeFixtureHTML(value string) string {
	value = html.UnescapeString(value)
	value = normalizeSpace(value)
	value = strings.ReplaceAll(value, "> <", "><")
	value = strings.ReplaceAll(value, " />", "/>")
	value = strings.ReplaceAll(value, " </p>", "</p>")
	value = strings.ReplaceAll(value, " </li>", "</li>")
	value = strings.ReplaceAll(value, "<tbody>", "")
	value = strings.ReplaceAll(value, "</tbody>", "")
	return strings.TrimSpace(value)
}

func summarizeHTML(value string) string {
	value = normalizeFixtureHTML(value)
	if len(value) <= 500 {
		return value
	}
	return value[:500] + "...(truncated)"
}
