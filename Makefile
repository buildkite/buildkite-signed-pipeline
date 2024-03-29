FLAGS = -s -w
FILES = $(shell find . -name '*.go')

.PHONY: build
build: dist/buildkite-signed-pipeline-linux-amd64 \
	dist/buildkite-signed-pipeline-linux-arm64 \
	dist/buildkite-signed-pipeline-windows-amd64.exe \
	dist/buildkite-signed-pipeline-darwin-amd64

dist/buildkite-signed-pipeline-linux-amd64: $(FILES)
	GOOS=linux GOARCH=amd64 go build -v -ldflags="$(FLAGS)" -o $@ ./cmd/buildkite-signed-pipeline

dist/buildkite-signed-pipeline-linux-arm64: $(FILES)
	GOOS=linux GOARCH=arm64 go build -v -ldflags="$(FLAGS)" -o $@ ./cmd/buildkite-signed-pipeline

dist/buildkite-signed-pipeline-windows-amd64.exe: $(FILES)
	GOOS=windows GOARCH=amd64 go build -v -ldflags="$(FLAGS)" -o $@ ./cmd/buildkite-signed-pipeline

dist/buildkite-signed-pipeline-darwin-amd64: $(FILES)
	GOOS=darwin GOARCH=amd64 go build -v -ldflags="$(FLAGS)" -o $@ ./cmd/buildkite-signed-pipeline

.PHONY: test
test:
	go test -v $(FILES)

.PHONY: clean
clean:
	rm -rf dist/
