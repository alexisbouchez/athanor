.PHONY: build test vet clean lint run run-workflow docs lima-create lima-start lima-stop lima-delete lima-shell

BINARY   := athanor
BUILD_DIR := bin
LIMA_VM   := athanor

# Build
build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/athanor

# Test
test:
	go test ./...

# Static analysis
vet:
	go vet ./...

# Lint (requires golangci-lint)
lint:
	golangci-lint run ./...

# Run TUI workflow selector
run: build
	./$(BUILD_DIR)/$(BINARY)

# Run a specific workflow file
run-workflow: build
	./$(BUILD_DIR)/$(BINARY) --workflow=$(WORKFLOW)

# Serve docs locally (requires: npm i -g mintlify)
docs:
	cd docs && mintlify dev

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)

# Lima VM management
lima-create:
	limactl create lima/athanor.yaml --name $(LIMA_VM)

lima-start:
	limactl start $(LIMA_VM)

lima-stop:
	limactl stop $(LIMA_VM)

lima-delete:
	limactl delete $(LIMA_VM)

lima-shell:
	limactl shell $(LIMA_VM)

# Cross-compile for Linux (to run inside the Lima VM)
build-linux:
	GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./cmd/athanor
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/athanor
