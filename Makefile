.PHONY: all test vet compat-test fuzz fuzz-from-reader fuzz-readerable bench bench-baseline bench-compare

all: vet test

test:
	go test -race -cover -count=1 ./...
	READABILITY_FULL_COMPAT=1 go test -cover -count=1 -run 'TestParseAllMozilla(Metadata|Content)Fixtures|TestMozilla' ./...

# compat-test runs only the Mozilla compatibility fixtures; useful when the
# default unit tests have already passed and you only want the drift report.
compat-test:
	READABILITY_FULL_COMPAT=1 go test -cover -count=1 -run 'TestParseAllMozilla(Metadata|Content)Fixtures|TestMozilla' ./...

vet:
	go vet ./...

# Short-duration fuzz pass for CI; bump FUZZTIME locally for deeper runs.
FUZZTIME ?= 30s

fuzz:
	go test -run=^$$ -fuzz=FuzzFromReader -fuzztime=$(FUZZTIME) .
	go test -run=^$$ -fuzz=FuzzIsProbablyReaderable -fuzztime=$(FUZZTIME) .

# Single-target fuzz runs for long local sessions. The combined `fuzz`
# target serializes the two harnesses, which wastes wall-clock when you
# only care about one of them. Run as e.g. `make fuzz-from-reader FUZZTIME=10m`.
fuzz-from-reader:
	go test -run=^$$ -fuzz=FuzzFromReader -fuzztime=$(FUZZTIME) .

fuzz-readerable:
	go test -run=^$$ -fuzz=FuzzIsProbablyReaderable -fuzztime=$(FUZZTIME) .

# Benchmark targets.
#
# `bench`           runs the suite once, prints results, leaves nothing on
#                   disk. Use during local iteration.
# `bench-baseline`  records a fresh testdata/bench-baseline.txt with enough
#                   samples for benchstat confidence intervals (count=6).
#                   Run this after a deliberate perf change to update the
#                   committed baseline.
# `bench-compare`   runs the suite into /tmp/bench-current.txt and diffs it
#                   against the committed baseline via benchstat. Exits
#                   non-zero only on benchstat invocation errors; perf
#                   regressions are reported as part of the textual output
#                   so reviewers can decide whether the drift is acceptable.
BENCHTIME ?= 2s
BENCHCOUNT ?= 6

bench:
	go test -bench=. -benchmem -benchtime=$(BENCHTIME) -count=1 -run=^$$ .

bench-baseline:
	go test -bench=. -benchmem -benchtime=$(BENCHTIME) -count=$(BENCHCOUNT) -run=^$$ . \
		> testdata/bench-baseline.txt

bench-compare:
	@command -v benchstat >/dev/null 2>&1 || { \
		echo "benchstat not found; install with: go install golang.org/x/perf/cmd/benchstat@latest"; \
		exit 1; \
	}
	go test -bench=. -benchmem -benchtime=$(BENCHTIME) -count=$(BENCHCOUNT) -run=^$$ . \
		> /tmp/bench-current.txt
	benchstat testdata/bench-baseline.txt /tmp/bench-current.txt
