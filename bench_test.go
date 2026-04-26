package readability

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// benchFixtures lists representative fixtures spanning small / medium / large
// documents so benchmarks reflect realistic input variance.
var benchFixtures = []string{
	"001",                  // small (~12KB)
	"wikipedia",            // large MediaWiki article
	"nytimes-1",            // typical news article
	"medium-1",             // blog platform
	"hidden-nodes",         // visibility-heavy
}

func loadBenchFixture(b *testing.B, name string) []byte {
	b.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "test-pages", name, "source.html"))
	if err != nil {
		b.Skipf("fixture %s unavailable: %v", name, err)
	}
	return data
}

func BenchmarkFromReader(b *testing.B) {
	for _, name := range benchFixtures {
		data := loadBenchFixture(b, name)
		if data == nil {
			continue
		}
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := FromReader(bytes.NewReader(data), "", nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkIsProbablyReaderable(b *testing.B) {
	for _, name := range benchFixtures {
		data := loadBenchFixture(b, name)
		if data == nil {
			continue
		}
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := IsProbablyReaderable(bytes.NewReader(data)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
