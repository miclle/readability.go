#!/usr/bin/env bash
# bench-regression-gate.sh: parse benchstat CSV output and fail when any
# significant benchmark regresses past the configured threshold.
#
# Significance is determined by benchstat itself (p < 0.05 by default; the
# "vs base" column reads "~" when the difference is treated as noise). We
# only consider benchmarks whose "vs base" column shows a positive
# percent change (regression in sec/op or B/op) AND benchstat marked the
# change as significant. Improvements ("-X.XX%") and noise ("~") are
# ignored.
#
# Usage: bench-regression-gate.sh <benchstat-csv> <threshold-percent>
# Threshold is the maximum tolerated regression (e.g. 5 means "fail on
# +5.00% or more").

set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <benchstat-csv> <threshold-percent>" >&2
  exit 64
fi

csv=$1
threshold=$2

if [[ ! -s "$csv" ]]; then
  echo "bench CSV is empty: $csv" >&2
  exit 65
fi

# benchstat CSV layout (with one base + one new file):
#   name, base sec/op, CI, new sec/op, CI, vs base, P
# Column 6 is "vs base"; "~" means not significant. We strip the leading
# header rows (those start with "goos:"/"goarch:"/"pkg:" or contain
# field labels rather than benchmark rows) by requiring numeric data in
# column 4.

regressions=$(awk -F',' -v threshold="$threshold" '
  /^goos:/ || /^goarch:/ || /^pkg:/ { next }
  $1 == "" { next }                   # skip the header rows benchstat emits
  $4 ~ /^[0-9]/ {                      # numeric new sec/op column
    vs = $6
    if (vs == "~" || vs == "") next   # not significant or geomean row
    sign = substr(vs, 1, 1)
    if (sign != "+") next             # negative = improvement, skip
    pct = vs
    sub(/%$/, "", pct)
    sub(/^\+/, "", pct)
    if (pct + 0.0 >= threshold + 0.0) {
      printf "%s\t%s\n", $1, vs
    }
  }
' "$csv")

if [[ -n "$regressions" ]]; then
  echo "Bench regressions exceeding ${threshold}% threshold:" >&2
  echo "$regressions" >&2
  exit 1
fi

echo "No bench regressions exceeding ${threshold}% threshold."
