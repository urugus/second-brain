BINARY := sb
VERSION := 0.1.0
LDFLAGS := -ldflags "-s -w -X github.com/urugus/second-brain/cmd.Version=$(VERSION)"

.PHONY: build build-all test lint clean

build:
	go build $(LDFLAGS) -o $(BINARY) .

build-all:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-arm64 .
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-arm64 .
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-amd64 .

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -f $(BINARY)
	rm -rf dist/
