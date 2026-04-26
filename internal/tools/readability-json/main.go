package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	readability "github.com/miclle/readability.go"
)

func main() {
	pageURL := flag.String("url", "http://fakehost/test/", "page URL")
	charThreshold := flag.Int("char-threshold", 500, "minimum extracted text length")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: readability-json [flags] source.html")
		os.Exit(2)
	}

	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	article, err := readability.FromReader(f, *pageURL, &readability.Options{CharThreshold: *charThreshold})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := json.NewEncoder(os.Stdout).Encode(article); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
