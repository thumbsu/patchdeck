REPO ?=
BIN_DIR ?= $(HOME)/.local/bin

.PHONY: test build run list install

test:
	go test ./...

build:
	go build -o patchdeck ./cmd/patchdeck

run:
	go run ./cmd/patchdeck $(if $(REPO),--repo $(REPO),)

list:
	go run ./cmd/patchdeck --list $(if $(REPO),--repo $(REPO),)

install:
	mkdir -p "$(BIN_DIR)"
	go build -o "$(BIN_DIR)/patchdeck" ./cmd/patchdeck
	@echo "Installed to $(BIN_DIR)/patchdeck"
