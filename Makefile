.PHONY: build test lint fmt tidy clean

build:
	go build -o social-timeline ./cmd/social-timeline

test:
	go test ./...

lint:
	go tool golangci-lint run

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

clean:
	rm -f social-timeline
