.PHONY: test compat-test strict-compat-test vet

test:
	go test ./...

compat-test:
	READABILITY_FULL_COMPAT=1 go test -run 'TestParseAllMozilla(Metadata|Content)Fixtures|TestMozilla' ./...

strict-compat-test:
	@echo "strict fixture matching is not a maintenance gate; use 'make compat-test' to inspect compatibility drift"

vet:
	go vet ./...
