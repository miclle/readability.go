package readability

import (
	"errors"
	"strings"
	"testing"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func TestIsProbablyReaderableOptionsAndFilters(t *testing.T) {
	html := `<html><body>
		<p style="display:none">this hidden paragraph is long enough but invisible</p>
		<div class="comment"><p>this unlikely paragraph is long enough but should be ignored</p></div>
		<ul><li><p>this list paragraph is long enough but should be ignored</p></li></ul>
		<article><p>This visible article paragraph has enough text to be readerable.</p></article>
	</body></html>`

	ok, err := IsProbablyReaderable(strings.NewReader(html), ReaderableOptions{
		MinContentLength: 20,
		MinScore:         1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("visible article paragraph should be readerable")
	}
}

func TestIsProbablyReaderableRejectsNonArticleContent(t *testing.T) {
	html := `<html><body>
		<p hidden>This hidden paragraph should not count toward readerability.</p>
		<p class="comment">This comment paragraph should not count toward readerability.</p>
		<ul><li><p>This list paragraph should not count toward readerability.</p></li></ul>
	</body></html>`

	ok, err := IsProbablyReaderable(strings.NewReader(html), ReaderableOptions{
		MinContentLength: 20,
		MinScore:         1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("filtered content should not be readerable")
	}
}

func TestIsProbablyReaderableIncludesDivsWithBreaks(t *testing.T) {
	html := `<html><body><div>First line of an article-like block<br><br>Second line with enough useful text to pass.</div></body></html>`

	ok, err := IsProbablyReaderable(strings.NewReader(html), ReaderableOptions{
		MinContentLength: 20,
		MinScore:         1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("div with br-separated text should be readerable")
	}
}

func TestIsProbablyReaderableReturnsReaderError(t *testing.T) {
	ok, err := IsProbablyReaderable(failingReader{})
	if err == nil {
		t.Fatal("expected reader error")
	}
	if ok {
		t.Fatal("reader error should not be readerable")
	}
}
