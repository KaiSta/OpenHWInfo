# OpenHWInfo Makefile
# Usage: make build | make run | make dev | make test | make clean

BINARY  := OpenHWInfo
TAGS    := production,webkit2_41
DEV_TAGS := dev,webkit2_41

.PHONY: build run dev test clean

## Build the native binary (production)
build:
	go build -tags=$(TAGS) -o $(BINARY) .

## Run the app in production mode
run: build
	./$(BINARY)

## Run in development mode (hot-reload, serves frontend from disk)
dev:
	go run -tags=$(DEV_TAGS) .

## Run all tests
test:
	go test ./...

## Clean build artifacts
clean:
	rm -f $(BINARY)