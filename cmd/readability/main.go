package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/JohannesKaufmann/html-to-markdown/plugin"
	readability "github.com/miclle/readability.go"
)

const (
	defaultFormat    = "text"
	defaultTimeout   = 15 * time.Second
	defaultUserAgent = "readability.go/cli"
	maxHTTPBodyBytes = 10 << 20
)

type cliConfig struct {
	format        string
	pageURL       string
	charThreshold int
	timeout       time.Duration
	userAgent     string
	metadata      bool
	help          bool
	source        string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if cfg.help {
		fmt.Fprint(stdout, helpText())
		return 0
	}

	data, pageURL, err := readSource(cfg, stdin)
	if err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprintln(stderr, strings.TrimPrefix(err.Error(), errUsage.Error()+": "))
			return 2
		}
		fmt.Fprintln(stderr, err)
		return 1
	}

	article, err := readability.FromReader(bytes.NewReader(data), pageURL, &readability.Options{CharThreshold: cfg.charThreshold})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	output, err := renderOutput(article, cfg.format, cfg.metadata, pageURL)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if _, err := io.WriteString(stdout, output); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !strings.HasSuffix(output, "\n") {
		if _, err := io.WriteString(stdout, "\n"); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	return 0
}

func parseArgs(args []string) (cliConfig, error) {
	cfg := cliConfig{
		format:    defaultFormat,
		timeout:   defaultTimeout,
		userAgent: defaultUserAgent,
	}
	var sources []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" || arg == "--help" {
			cfg.help = true
			continue
		}
		if !strings.HasPrefix(arg, "--") || arg == "-" {
			sources = append(sources, arg)
			continue
		}

		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		switch name {
		case "format":
			var err error
			value, i, err = flagValue(args, i, value, hasValue, name)
			if err != nil {
				return cfg, err
			}
			cfg.format = value
		case "url":
			var err error
			value, i, err = flagValue(args, i, value, hasValue, name)
			if err != nil {
				return cfg, err
			}
			cfg.pageURL = value
		case "char-threshold":
			var err error
			value, i, err = flagValue(args, i, value, hasValue, name)
			if err != nil {
				return cfg, err
			}
			cfg.charThreshold, err = strconv.Atoi(value)
			if err != nil || cfg.charThreshold < 0 {
				return cfg, fmt.Errorf("--char-threshold must be a non-negative integer")
			}
		case "timeout":
			var err error
			value, i, err = flagValue(args, i, value, hasValue, name)
			if err != nil {
				return cfg, err
			}
			cfg.timeout, err = time.ParseDuration(value)
			if err != nil || cfg.timeout <= 0 {
				return cfg, fmt.Errorf("--timeout must be a positive duration")
			}
		case "user-agent":
			var err error
			value, i, err = flagValue(args, i, value, hasValue, name)
			if err != nil {
				return cfg, err
			}
			cfg.userAgent = value
		case "metadata":
			if hasValue {
				parsed, err := strconv.ParseBool(value)
				if err != nil {
					return cfg, fmt.Errorf("--metadata must be a boolean")
				}
				cfg.metadata = parsed
			} else {
				cfg.metadata = true
			}
		default:
			return cfg, fmt.Errorf("unknown flag --%s", name)
		}
	}
	if cfg.help {
		return cfg, nil
	}
	if len(sources) != 1 {
		return cfg, fmt.Errorf("usage: readability [flags] URL|FILE|-")
	}
	cfg.source = sources[0]

	format, err := normalizeFormat(cfg.format)
	if err != nil {
		return cfg, err
	}
	cfg.format = format
	if cfg.metadata && cfg.format != "markdown" {
		return cfg, fmt.Errorf("--metadata is only supported with --format markdown")
	}
	return cfg, nil
}

func flagValue(args []string, index int, value string, hasValue bool, name string) (string, int, error) {
	if hasValue {
		if value == "" {
			return "", index, fmt.Errorf("--%s requires a value", name)
		}
		return value, index, nil
	}
	if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
		return "", index, fmt.Errorf("--%s requires a value", name)
	}
	return args[index+1], index + 1, nil
}

func normalizeFormat(format string) (string, error) {
	switch strings.ToLower(format) {
	case "text":
		return "text", nil
	case "html":
		return "html", nil
	case "json":
		return "json", nil
	case "markdown", "md":
		return "markdown", nil
	default:
		return "", fmt.Errorf("unsupported format %q", format)
	}
}

func helpText() string {
	return `Usage: readability [flags] URL|FILE|-

Extract the readable article from a URL, HTML file, or stdin.

Arguments:
  URL                 Fetch an http or https page.
  FILE                Read HTML from a local file.
  -                   Read HTML from stdin.

Examples:
  readability https://example.com/post
  readability article.html --url https://example.com/post --format json
  cat article.html | readability - --url https://example.com/post --format md --metadata

Flags:
  --format value      Output format: text, html, json, markdown, md. Default: text.
  --url value         Base page URL for file and stdin input.
  --char-threshold n  Minimum extracted text length. Default: 0.
  --timeout duration  HTTP timeout for URL input. Default: 15s.
  --user-agent value  HTTP User-Agent for URL input. Default: readability.go/cli.
  --metadata          Add YAML front matter to Markdown output.
  -h, --help          Show this help.
`
}

var errUsage = errors.New("usage")

func readSource(cfg cliConfig, stdin io.Reader) ([]byte, string, error) {
	if cfg.source == "-" {
		data, err := io.ReadAll(stdin)
		return data, cfg.pageURL, err
	}

	parsed, err := url.Parse(cfg.source)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
		return fetchURL(cfg)
	}
	if err == nil && parsed.Scheme != "" && strings.Contains(cfg.source, "://") {
		return nil, "", fmt.Errorf("%w: unsupported URL scheme %q", errUsage, parsed.Scheme)
	}

	data, err := os.ReadFile(cfg.source)
	return data, cfg.pageURL, err
}

func fetchURL(cfg cliConfig) ([]byte, string, error) {
	client := &http.Client{Timeout: cfg.timeout}
	req, err := http.NewRequest(http.MethodGet, cfg.source, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", cfg.userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	if !isHTMLContentType(resp.Header.Get("Content-Type")) {
		return nil, "", fmt.Errorf("unsupported content type %q", resp.Header.Get("Content-Type"))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPBodyBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxHTTPBodyBytes {
		return nil, "", fmt.Errorf("response body exceeds %d bytes", maxHTTPBodyBytes)
	}
	return data, resp.Request.URL.String(), nil
}

func isHTMLContentType(value string) bool {
	if value == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}

func renderOutput(article readability.Article, format string, metadata bool, pageURL string) (string, error) {
	switch format {
	case "text":
		return article.TextContent, nil
	case "html":
		return article.Content, nil
	case "json":
		data, err := json.MarshalIndent(article, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	case "markdown":
		converter := md.NewConverter("", true, nil)
		converter.Use(plugin.GitHubFlavored())
		markdown, err := converter.ConvertString(article.Content)
		if err != nil {
			return "", err
		}
		markdown = strings.TrimSpace(markdown)
		if metadata {
			markdown = renderFrontMatter(article, pageURL) + markdown
		}
		return markdown, nil
	default:
		return "", fmt.Errorf("unsupported format %q", format)
	}
}

func renderFrontMatter(article readability.Article, pageURL string) string {
	fields := []struct {
		key   string
		value string
	}{
		{key: "title", value: article.Title},
		{key: "byline", value: article.Byline},
		{key: "site_name", value: article.SiteName},
		{key: "published_time", value: article.PublishedTime},
		{key: "url", value: pageURL},
	}

	var b strings.Builder
	b.WriteString("---\n")
	for _, field := range fields {
		if field.value == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", field.key, strconv.Quote(field.value))
	}
	b.WriteString("---\n\n")
	return b.String()
}
