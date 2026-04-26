#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const upstream = process.env.READABILITY_UPSTREAM || "/Users/miclle/mozilla/readability";
const goJSON = process.env.READABILITY_GO_JSON || "go";
const require = createRequire(import.meta.url);
const { Readability } = require(join(upstream, "index.js"));
const JSDOMParser = require(join(upstream, "JSDOMParser.js"));

const args = process.argv.slice(2);
const compareAll = args.includes("--all");
const charThreshold = numberOption("--char-threshold", 500);
const requested = args.filter((arg, index) => {
  if (arg === "--all" || arg === "--char-threshold") {
    return false;
  }
  if (args[index - 1] === "--char-threshold") {
    return false;
  }
  return !arg.startsWith("-");
});
const fixtureRoot = join(root, "testdata", "test-pages");
const fixtures = compareAll ? allFixtures() : requested.length > 0 ? requested : [
  "001",
  "004-metadata-space-separated-properties",
  "article-author-tag",
  "base-url",
  "cnet-svg-classes",
  "embedded-videos",
  "keep-tabular-data",
  "nytimes-2",
  "rtl-1",
  "wikipedia-4",
];

const fields = ["title", "byline", "excerpt", "siteName", "publishedTime", "dir", "textContent"];
const maxText = 180;
let mismatches = 0;

for (const fixture of fixtures) {
  const sourcePath = join(fixtureRoot, fixture, "source.html");
  const source = readFileSync(sourcePath, "utf8");
  const url = "http://fakehost/test/";
  const upstreamArticle = parseUpstream(source, url);
  const goArticle = parseGo(sourcePath, url);
  const diffs = compareArticles(upstreamArticle, goArticle);
  if (diffs.length === 0) {
    console.log(`ok ${fixture}`);
    continue;
  }
  mismatches++;
  console.log(`diff ${fixture}`);
  for (const diff of diffs) {
    const detail = typeof diff.js === "string" && typeof diff.go === "string"
      ? ` len(js/go)=${diff.js.length}/${diff.go.length} firstDiff=${firstDiffIndex(diff.js, diff.go)}`
      : "";
    console.log(`  ${diff.field}:${detail}`);
    console.log(`    js: ${formatValue(diff.js)}`);
    console.log(`    go: ${formatValue(diff.go)}`);
  }
}

if (mismatches > 0) {
  process.exitCode = 1;
}

function parseUpstream(source, url) {
	const doc = new JSDOMParser().parse(source, url);
	const article = new Readability(doc, { classesToPreserve: ["caption"], charThreshold }).parse();
	return article || {};
}

function allFixtures() {
  return readdirSync(fixtureRoot, { withFileTypes: true })
    .filter(entry => entry.isDirectory())
    .map(entry => entry.name)
    .sort();
}

function parseGo(sourcePath, url) {
  const stdout = execFileSync(goJSON, [
	 ...goJSONArgs(),
	 "-url",
	 url,
	 "-char-threshold",
	 String(charThreshold),
	 sourcePath,
  ], {
    cwd: root,
    encoding: "utf8",
    maxBuffer: 20 * 1024 * 1024,
  });
  const article = JSON.parse(stdout);
  return {
    title: article.Title,
    byline: article.Byline || null,
    excerpt: article.Excerpt,
    siteName: article.SiteName || null,
    publishedTime: article.PublishedTime || null,
    dir: article.Dir || null,
    content: article.Content,
    textContent: article.TextContent,
  };
}

function goJSONArgs() {
  if (goJSON === "go") {
    return ["run", "./internal/tools/readability-json"];
  }
  return [];
}

function compareArticles(jsArticle, goArticle) {
  const diffs = [];
  for (const field of fields) {
    const jsValue = normalizeField(field, jsArticle[field] ?? null);
    const goValue = normalizeField(field, goArticle[field] ?? null);
    if (jsValue !== goValue) {
      diffs.push({ field, js: jsValue, go: goValue });
    }
  }
	if (normalizeHTML(jsArticle.content || "") !== normalizeHTML(goArticle.content || "")) {
		diffs.push({ field: "content", js: normalizeHTML(jsArticle.content || ""), go: normalizeHTML(goArticle.content || "") });
	}
	return diffs;
}

function numberOption(name, fallback) {
  const index = args.indexOf(name);
  if (index === -1) {
    return fallback;
  }
  const value = Number(args[index + 1]);
  if (!Number.isFinite(value)) {
    throw new Error(`${name} requires a number`);
  }
  return value;
}

function normalizeField(field, value) {
  if (value === undefined || value === "") {
    value = null;
  }
  if (typeof value !== "string") {
    return value;
  }
  const normalized = value.replace(/\s+/g, " ").trim();
  if (field === "textContent") {
    return normalized;
  }
  return normalized || null;
}

function normalizeHTML(value) {
  return value
    .replace(/>\s+</g, "><")
    .replace(/\s+/g, " ")
    .replace(/ \/>/g, "/>")
    .trim();
}

function summarizeHTML(value) {
  const normalized = normalizeHTML(value);
  return normalized.length > maxText ? `${normalized.slice(0, maxText)}...` : normalized;
}

function formatValue(value) {
  if (value === null) {
    return "null";
  }
  const text = String(value);
  return text.length > maxText ? `${text.slice(0, maxText)}...` : text;
}

function firstDiffIndex(a, b) {
  const limit = Math.min(a.length, b.length);
  for (let i = 0; i < limit; i++) {
    if (a[i] !== b[i]) {
      return i;
    }
  }
  return a.length === b.length ? -1 : limit;
}
