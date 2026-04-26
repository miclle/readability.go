.PHONY: all test vet compat-test fuzz

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
