package readability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMozillaFixture001(t *testing.T) {
	sourcePath := filepath.Join("testdata", "mozilla", "test-pages", "001", "source.html")
	f, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	article, err := FromReader(f, "http://fakehost/code/2013/get-your-frontend-javascript-code-covered/", nil)
	if err != nil {
		t.Fatal(err)
	}

	if article.Title != "Get your Frontend JavaScript Code Covered | Code" {
		t.Fatalf("Title = %q", article.Title)
	}
	if article.Byline != "Nicolas Perriault" {
		t.Fatalf("Byline = %q", article.Byline)
	}
	if article.Excerpt != "Nicolas Perriault's homepage." {
		t.Fatalf("Excerpt = %q", article.Excerpt)
	}
	if article.Lang != "en" {
		t.Fatalf("Lang = %q", article.Lang)
	}
	if !strings.Contains(article.TextContent, "testing your frontend JavaScript code") {
		t.Fatalf("TextContent does not contain expected article text")
	}
	if !strings.Contains(article.Content, `<div id="readability-page-1" class="page">`) {
		t.Fatalf("Content does not contain readability page wrapper")
	}
}

func TestIsProbablyReaderableMozillaFixture001(t *testing.T) {
	sourcePath := filepath.Join("testdata", "mozilla", "test-pages", "001", "source.html")
	f, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	ok, err := IsProbablyReaderable(f)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("fixture 001 should be readerable")
	}
}
