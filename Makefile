.PHONY: test compat-test vet

test:
	go test ./...

compat-test:
	READABILITY_FULL_COMPAT=1 go test -run 'TestParseAllMozilla(Metadata|Content)Fixtures|TestMozilla' ./...

vet:
	go vet ./...
