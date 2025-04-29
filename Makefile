# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
BINARY_NAME=monibuca
MAIN_PATH=./example/default
TAGS=sqlite

# Output directory
BUILD_DIR=build

# Version info
VERSION=$(shell git describe --tags --always || echo "unknown")
BUILD_TIME=$(shell date +%FT%T%z)

# Build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

.PHONY: all build clean windows linux golinux crosslinux

all: clean build

build: windows linux

# Create build directory
$(BUILD_DIR):
	if not exist $(BUILD_DIR) mkdir $(BUILD_DIR)

# Clean
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

# Windows build
windows:
	@echo "Building Windows version..."
	if not exist $(BUILD_DIR) mkdir $(BUILD_DIR)
	$(GOBUILD) -tags $(TAGS) -v -o $(BUILD_DIR)/$(BINARY_NAME).exe $(MAIN_PATH)
	@echo "Windows build completed"

# Linux build
linux:
	@echo "Building Linux version..."
	if not exist $(BUILD_DIR) mkdir $(BUILD_DIR)
	$(GOBUILD) -tags "$(TAGS) netgo" -a -v -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Linux build completed"

# Direct go command for Linux (without Make environment variables)
golinux:
	@echo "Building Linux version using direct Go command..."
	if not exist $(BUILD_DIR) mkdir $(BUILD_DIR)
	go build -tags "$(TAGS) netgo" -a -v -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Linux build completed"

# Cross-compile for Linux
crosslinux:
	@echo "Cross-compiling for Linux..."
	if not exist $(BUILD_DIR) mkdir $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags "$(TAGS) netgo" -a -v -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Linux cross-compilation completed"

# Help
help:
	@echo "Available make commands:"
	@echo "  make all        - Clean and build all platforms"
	@echo "  make windows    - Build Windows version only"
	@echo "  make linux      - Build Linux version only"
	@echo "  make golinux    - Build Linux version using direct Go command"
	@echo "  make crosslinux - Cross-compile for Linux"
	@echo "  make clean      - Clean build files"
	@echo ""
	@echo "Note: All builds include the 'sqlite' tag"
