.PHONY: build run dev clean test

APP_NAME := newmcp
BUILD_DIR := build

build:
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server/

run: build
	./$(BUILD_DIR)/$(APP_NAME)

dev:
	go run ./cmd/server/

clean:
	rm -rf $(BUILD_DIR)/ data/

test:
	go test ./... -v

tidy:
	go mod tidy
