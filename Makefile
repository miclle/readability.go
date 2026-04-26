.PHONY: all test vet

all: vet test

test:
	go test -race -cover -count=1 ./...
	READABILITY_FULL_COMPAT=1 go test -cover -count=1 -run 'TestParseAllMozilla(Metadata|Content)Fixtures|TestMozilla' ./...

vet:
	go vet ./...
