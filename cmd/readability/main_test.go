package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	readability "github.com/miclle/readability.go"
	"gopkg.in/yaml.v2"
)

const testHTML = `<!doctype html>
<html>
<head>
<title>CLI Article</title>
<meta name="author" content="Jane Doe">
<meta property="og:site_name" content="Example News">
<meta property="article:published_time" content="2026-04-28T12:00:00Z">
</head>
<body>
<article>
<h1>CLI Article</h1>
<p>This article body contains enough meaningful words for the readability command to extract it as the main document content without falling below the parser heuristics.</p>
<p>A second substantial paragraph keeps the article candidate stable and includes <a href="/related">a related link</a> plus an image.</p>
<img src="/hero.jpg" alt="Hero image">
<table><thead><tr><th>Name</th><th>Value</th></tr></thead><tbody><tr><td>Alpha</td><td>One</td></tr></tbody></table>
</article>
</body>
</html>`

func TestRunFileInputOutputsTextByDefault(t *testing.T) {
	path := writeTempHTML(t, testHTML)
	stdout, stderr, code := runCommand(t, []string{path})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "This article body contains enough meaningful words") {
		t.Fatalf("stdout does not contain article text: %q", stdout)
	}
	if strings.Contains(stdout, "<article>") {
		t.Fatalf("text output contains HTML: %q", stdout)
	}
}

func TestRunFileInputAllowsColonInPath(t *testing.T) {
	workingDir := t.TempDir()
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.WriteFile("draft:v1.html", []byte(testHTML), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCommand(t, []string{"draft:v1.html"})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "This article body contains enough meaningful words") {
		t.Fatalf("stdout does not contain article text: %q", stdout)
	}
}

func TestRunStdinHTMLResolvesRelativeLinks(t *testing.T) {
	stdout, stderr, code := runCommandWithInput(t, []string{"-", "--url", "https://example.com/post", "--format", "html"}, testHTML)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, `href="https://example.com/related"`) {
		t.Fatalf("html output did not resolve href: %q", stdout)
	}
	if !strings.Contains(stdout, `src="https://example.com/hero.jpg"`) {
		t.Fatalf("html output did not resolve src: %q", stdout)
	}
}

func TestRunJSONOutputIncludesArticleFields(t *testing.T) {
	path := writeTempHTML(t, testHTML)
	stdout, stderr, code := runCommand(t, []string{path, "--url", "https://example.com/post", "--format", "json"})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	var article readability.Article
	if err := json.Unmarshal([]byte(stdout), &article); err != nil {
		t.Fatalf("json output did not decode: %v\n%s", err, stdout)
	}
	if article.Title != "CLI Article" {
		t.Fatalf("Title = %q", article.Title)
	}
	if article.SiteName != "Example News" {
		t.Fatalf("SiteName = %q", article.SiteName)
	}
	if article.Length == 0 {
		t.Fatal("Length = 0")
	}
}

func TestRunMarkdownOutputWithMetadata(t *testing.T) {
	stdout, stderr, code := runCommandWithInput(t, []string{"-", "--url", "https://example.com/post", "--format", "md", "--metadata"}, testHTML)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	for _, want := range []string{
		"---\n",
		`title: "CLI Article"`,
		`byline: "Jane Doe"`,
		`site_name: "Example News"`,
		`published_time: "2026-04-28T12:00:00Z"`,
		`url: "https://example.com/post"`,
		"This article body contains enough meaningful words",
		"[a related link](https://example.com/related)",
		"![Hero image](https://example.com/hero.jpg)",
		"| Name | Value |",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("markdown output missing %q:\n%s", want, stdout)
		}
	}
}

func TestRunMarkdownMetadataEscapesYAMLValues(t *testing.T) {
	html := strings.Replace(testHTML, "<title>CLI Article</title>", "<title>ACME: quarterly update</title>", 1)
	html = strings.Replace(html, `<meta name="author" content="Jane Doe">`, `<meta name="author" content="Jane # Docs">`, 1)

	stdout, stderr, code := runCommandWithInput(t, []string{"-", "--url", "https://example.com/post", "--format", "md", "--metadata"}, html)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	frontMatter := strings.SplitN(stdout, "---\n\n", 2)[0] + "---\n"
	var metadata map[string]string
	if err := yaml.Unmarshal([]byte(frontMatter), &metadata); err != nil {
		t.Fatalf("front matter did not parse as YAML: %v\n%s", err, frontMatter)
	}
	if metadata["title"] != "ACME: quarterly update" {
		t.Fatalf("title = %q", metadata["title"])
	}
	if metadata["byline"] != "Jane # Docs" {
		t.Fatalf("byline = %q", metadata["byline"])
	}
}

func TestRunURLInputFetchesHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "readability.go/cli" {
			t.Fatalf("User-Agent = %q", r.UserAgent())
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(testHTML))
	}))
	defer server.Close()

	stdout, stderr, code := runCommand(t, []string{server.URL, "--format", "json"})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, `"Title": "CLI Article"`) {
		t.Fatalf("json output missing title: %q", stdout)
	}
}

func TestRunUsageErrorsExitTwo(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing source", args: nil},
		{name: "invalid format", args: []string{"-", "--format", "xml"}},
		{name: "unsupported url scheme", args: []string{"ftp://example.com/post"}},
		{name: "metadata with html", args: []string{"-", "--format", "html", "--metadata"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, code := runCommandWithInput(t, tt.args, testHTML)
			if code != 2 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
			if stderr == "" {
				t.Fatal("stderr is empty")
			}
		})
	}
}

func TestRunHelpExitsZero(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := runCommand(t, args)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q", stderr)
			}
			for _, want := range []string{
				"Usage: readability [flags] URL|FILE|-",
				"Examples:",
				"readability https://example.com/post",
				"readability article.html --url https://example.com/post --format json",
				"cat article.html | readability - --url https://example.com/post --format md --metadata",
				"--format",
				"--metadata",
				"text, html, json, markdown",
			} {
				if !strings.Contains(stdout, want) {
					t.Fatalf("help output missing %q:\n%s", want, stdout)
				}
			}
		})
	}
}

func writeTempHTML(t *testing.T, html string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "article.html")
	if err := os.WriteFile(path, []byte(html), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func runCommand(t *testing.T, args []string) (string, string, int) {
	t.Helper()
	return runCommandWithInput(t, args, "")
}

func runCommandWithInput(t *testing.T, args []string, input string) (string, string, int) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(args, strings.NewReader(input), &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}
