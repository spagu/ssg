# Makefile for SSG (Static Site Generator)

# Colors
ifneq (,$(findstring xterm,${TERM}))
   BLACK        := $(shell tput -Txterm setaf 0)
   RED          := $(shell tput -Txterm setaf 1)
   GREEN        := $(shell tput -Txterm setaf 2)
   YELLOW       := $(shell tput -Txterm setaf 3)
   LIGHTPURPLE  := $(shell tput -Txterm setaf 4)
   PURPLE       := $(shell tput -Txterm setaf 5)
   BLUE         := $(shell tput -Txterm setaf 6)
   WHITE        := $(shell tput -Txterm setaf 7)
   RESET := $(shell tput -Txterm sgr0)
else
   BLACK        := ""
   RED          := ""
   GREEN        := ""
   YELLOW       := ""
   LIGHTPURPLE  := ""
   PURPLE       := ""
   BLUE         := ""
   WHITE        := ""
   RESET        := ""
endif

# Variables
BINARY_NAME=ssg
BUILD_DIR=build
CMD_DIR=cmd/ssg
GO=go
GOFLAGS=-v
LDFLAGS=-s -w

.PHONY: all build clean test lint run help deps tidy generate

# Default target
all: deps lint test build ## 🚀 Run all: deps, lint, test, build

help: ## 📖 Show this help message
	@echo "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
	@echo "${GREEN}  SSG - Static Site Generator${RESET}"
	@echo "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
	@echo ""
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  ${YELLOW}%-20s${RESET} %s\n", $$1, $$2}'
	@echo ""

# Dependencies
deps: ## 📦 Download dependencies
	@echo "${BLUE}📦 Downloading dependencies...${RESET}"
	@$(GO) mod download
	@echo "${GREEN}✅ Dependencies downloaded${RESET}"

tidy: ## 🧹 Tidy go modules
	@echo "${BLUE}🧹 Tidying go modules...${RESET}"
	@$(GO) mod tidy
	@echo "${GREEN}✅ Modules tidied${RESET}"

# Build
build: ## 🔨 Build the binary
	@echo "${BLUE}🔨 Building $(BINARY_NAME)...${RESET}"
	@mkdir -p $(BUILD_DIR)
	@$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "${GREEN}✅ Binary built: $(BUILD_DIR)/$(BINARY_NAME)${RESET}"

build-linux: ## 🐧 Build for Linux
	@echo "${BLUE}🐧 Building for Linux...${RESET}"
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	@echo "${GREEN}✅ Linux binary built${RESET}"

build-darwin: ## 🍎 Build for macOS
	@echo "${BLUE}🍎 Building for macOS...${RESET}"
	@mkdir -p $(BUILD_DIR)
	@GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./$(CMD_DIR)
	@GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./$(CMD_DIR)
	@echo "${GREEN}✅ macOS binaries built${RESET}"

build-windows: ## 🪟 Build for Windows
	@echo "${BLUE}🪟 Building for Windows...${RESET}"
	@mkdir -p $(BUILD_DIR)
	@GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)
	@echo "${GREEN}✅ Windows binary built${RESET}"

build-all: build-linux build-darwin build-windows ## 🌍 Build for all platforms

# Testing
test: ## 🧪 Run tests
	@echo "${BLUE}🧪 Running tests...${RESET}"
	@$(GO) test -v -race -coverprofile=coverage.out ./...
	@echo "${GREEN}✅ Tests passed${RESET}"

test-coverage: test ## 📊 Run tests with coverage report
	@echo "${BLUE}📊 Generating coverage report...${RESET}"
	@$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "${GREEN}✅ Coverage report: coverage.html${RESET}"

# Linting
lint: ## 🔍 Run linter
	@echo "${BLUE}🔍 Running linter...${RESET}"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "${YELLOW}⚠️  golangci-lint not installed, running go vet...${RESET}"; \
		$(GO) vet ./...; \
	fi
	@echo "${GREEN}✅ Linting complete${RESET}"

# Run
run: build ## ▶️  Build and run with example
	@echo "${BLUE}▶️  Running SSG...${RESET}"
	@./$(BUILD_DIR)/$(BINARY_NAME) krowy.net.2026-01-13110345 krowy krowy.net

generate: build ## 🏗️  Generate site with krowy template
	@echo "${BLUE}🏗️  Generating site...${RESET}"
	@./$(BUILD_DIR)/$(BINARY_NAME) krowy.net.2026-01-13110345 krowy krowy.net
	@echo "${GREEN}✅ Site generated in output/${RESET}"

generate-simple: build ## 🏗️  Generate site with simple template
	@echo "${BLUE}🏗️  Generating site with simple template...${RESET}"
	@./$(BUILD_DIR)/$(BINARY_NAME) krowy.net.2026-01-13110345 simple krowy.net
	@echo "${GREEN}✅ Site generated in output/${RESET}"

serve: generate ## 🌐 Generate and serve site locally
	@echo "${BLUE}🌐 Starting local server on http://localhost:8888${RESET}"
	@cd output && python3 -m http.server 8888

deploy: build ## ☁️  Generate site with ZIP for Cloudflare Pages deployment
	@echo "${BLUE}☁️  Generating deployment package...${RESET}"
	@./$(BUILD_DIR)/$(BINARY_NAME) krowy.net.2026-01-13110345 krowy krowy.net --webp --zip
	@echo "${GREEN}✅ Deployment package created: krowy.net.zip${RESET}"
	@echo "${YELLOW}📤 Upload krowy.net.zip to Cloudflare Pages${RESET}"

# Clean
clean: ## 🗑️  Clean build artifacts
	@echo "${BLUE}🗑️  Cleaning...${RESET}"
	@rm -rf $(BUILD_DIR)
	@rm -rf output
	@rm -f coverage.out coverage.html
	@rm -f *.zip
	@echo "${GREEN}✅ Cleaned${RESET}"

# Install
install: build ## 💿 Install binary to /usr/local/bin
	@echo "${BLUE}💿 Installing $(BINARY_NAME)...${RESET}"
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "${GREEN}✅ Installed to /usr/local/bin/$(BINARY_NAME)${RESET}"

uninstall: ## 🗑️  Uninstall binary
	@echo "${BLUE}🗑️  Uninstalling $(BINARY_NAME)...${RESET}"
	@sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "${GREEN}✅ Uninstalled${RESET}"
