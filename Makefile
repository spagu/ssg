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
# Single source of truth for the version (audit DOC-005).
VERSION := $(shell cat VERSION 2>/dev/null)
LDFLAGS=-s -w -X main.Version=$(VERSION)

.PHONY: all help deps tidy version-sync version-check \
        build build-linux build-freebsd build-darwin build-windows build-openbsd build-all \
        package-all package-deb package-rpm package-snap \
        test test-coverage lint security run generate generate-simple serve deploy \
        clean install uninstall release test-action

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

version-sync: ## 🔖 Propagate ./VERSION into all packaging manifests (DOC-005)
	@bash scripts/sync-version.sh

version-check: ## 🔎 Fail if any packaging manifest drifts from ./VERSION
	@bash scripts/sync-version.sh --check

# Build
build: ## 🔨 Build the binary
	@echo "${BLUE}🔨 Building $(BINARY_NAME)...${RESET}"
	@mkdir -p $(BUILD_DIR)
	@$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "${GREEN}✅ Binary built: $(BUILD_DIR)/$(BINARY_NAME)${RESET}"

build-linux: ## 🐧 Build for Linux (amd64 + arm64)
	@echo "${BLUE}🐧 Building for Linux...${RESET}"
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	@GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./$(CMD_DIR)
	@echo "${GREEN}✅ Linux binaries built${RESET}"

build-freebsd: ## 😈 Build for FreeBSD (amd64 + arm64)
	@echo "${BLUE}😈 Building for FreeBSD...${RESET}"
	@mkdir -p $(BUILD_DIR)
	@GOOS=freebsd GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-freebsd-amd64 ./$(CMD_DIR)
	@GOOS=freebsd GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-freebsd-arm64 ./$(CMD_DIR)
	@echo "${GREEN}✅ FreeBSD binaries built${RESET}"

build-darwin: ## 🍎 Build for macOS (amd64 + arm64)
	@echo "${BLUE}🍎 Building for macOS...${RESET}"
	@mkdir -p $(BUILD_DIR)
	@GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./$(CMD_DIR)
	@GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./$(CMD_DIR)
	@echo "${GREEN}✅ macOS binaries built${RESET}"

build-windows: ## 🪟 Build for Windows (amd64 + arm64)
	@echo "${BLUE}🪟 Building for Windows...${RESET}"
	@mkdir -p $(BUILD_DIR)
	@GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)
	@GOOS=windows GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe ./$(CMD_DIR)
	@echo "${GREEN}✅ Windows binaries built${RESET}"

build-all: build-linux build-freebsd build-darwin build-windows build-openbsd ## 🌍 Build for all platforms

build-openbsd: ## 🐡 Build for OpenBSD (amd64 + arm64)
	@echo "${BLUE}🐡 Building for OpenBSD...${RESET}"
	@mkdir -p $(BUILD_DIR)
	@GOOS=openbsd GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-openbsd-amd64 ./$(CMD_DIR)
	@GOOS=openbsd GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-openbsd-arm64 ./$(CMD_DIR)
	@echo "${GREEN}✅ OpenBSD binaries built${RESET}"

# Packaging
package-all: ## 📦 Build all packages (DEB, RPM, Snap)
	@echo "${BLUE}📦 Building all packages...${RESET}"
	@./packaging/scripts/build-all.sh all

package-deb: build-linux ## 📦 Build Debian packages (amd64 + arm64)
	@echo "${BLUE}📦 Building DEB packages...${RESET}"
	@./packaging/scripts/build-all.sh deb

package-rpm: build-linux ## 📦 Build RPM packages (amd64 + arm64)
	@echo "${BLUE}📦 Building RPM packages...${RESET}"
	@./packaging/scripts/build-all.sh rpm

package-snap: ## 📦 Build Snap package
	@echo "${BLUE}📦 Building Snap package...${RESET}"
	@snapcraft

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

security: ## 🔒 Run SAST + vulnerability scan (gosec + govulncheck)
	@echo "${BLUE}🔒 Running gosec...${RESET}"
	@if command -v gosec >/dev/null 2>&1; then gosec -quiet ./...; else echo "${YELLOW}⚠️  gosec not installed (go install github.com/securego/gosec/v2/cmd/gosec@latest)${RESET}"; fi
	@echo "${BLUE}🔒 Running govulncheck...${RESET}"
	@if command -v govulncheck >/dev/null 2>&1; then govulncheck ./...; else echo "${YELLOW}⚠️  govulncheck not installed (go install golang.org/x/vuln/cmd/govulncheck@latest)${RESET}"; fi
	@echo "${GREEN}✅ Security scan complete${RESET}"

# Run
run: build ## ▶️  Build and run with example
	@echo "${BLUE}▶️  Running SSG...${RESET}"
	@./$(BUILD_DIR)/$(BINARY_NAME) test-content krowy example.com

generate: build ## 🏗️  Generate site with krowy template
	@echo "${BLUE}🏗️  Generating site...${RESET}"
	@./$(BUILD_DIR)/$(BINARY_NAME) test-content krowy example.com
	@echo "${GREEN}✅ Site generated in output/${RESET}"

generate-simple: build ## 🏗️  Generate site with simple template
	@echo "${BLUE}🏗️  Generating site with simple template...${RESET}"
	@./$(BUILD_DIR)/$(BINARY_NAME) test-content simple example.com
	@echo "${GREEN}✅ Site generated in output/${RESET}"

serve: generate ## 🌐 Generate and serve site locally
	@echo "${BLUE}🌐 Starting local server on http://localhost:8888${RESET}"
	@cd output && python3 -m http.server 8888

deploy: build ## ☁️  Generate site with ZIP for Cloudflare Pages deployment
	@echo "${BLUE}☁️  Generating deployment package...${RESET}"
	@./$(BUILD_DIR)/$(BINARY_NAME) test-content krowy example.com --webp --zip
	@echo "${GREEN}✅ Deployment package created: example.com.zip${RESET}"
	@echo "${YELLOW}📤 Upload example.com.zip to Cloudflare Pages${RESET}"

# Clean
clean: ## 🗑️  Clean build artifacts
	@echo "${BLUE}🗑️  Cleaning...${RESET}"
	@rm -rf $(BUILD_DIR)
	@rm -rf output
	@rm -f coverage.out coverage.html
	@rm -f *.zip
	@echo "${GREEN}✅ Cleaned${RESET}"

# Install
install: build ## 💿 Install binary and man page
	@echo "${BLUE}💿 Installing $(BINARY_NAME)...${RESET}"
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@if [ -f man/ssg.1 ]; then \
		sudo mkdir -p /usr/local/share/man/man1; \
		sudo cp man/ssg.1 /usr/local/share/man/man1/ssg.1; \
		echo "${GREEN}✅ Man page installed${RESET}"; \
	fi
	@echo "${GREEN}✅ Installed to /usr/local/bin/$(BINARY_NAME)${RESET}"

uninstall: ## 🗑️  Uninstall binary and man page
	@echo "${BLUE}🗑️  Uninstalling $(BINARY_NAME)...${RESET}"
	@sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@sudo rm -f /usr/local/share/man/man1/ssg.1
	@echo "${GREEN}✅ Uninstalled${RESET}"

# Release
release: ## 🏷️  Create a new release tag (usage: make release VERSION=v1.2.0)
ifndef VERSION
	@echo "${RED}❌ VERSION is required. Usage: make release VERSION=v1.2.0${RESET}"
	@exit 1
endif
	@echo "${BLUE}🏷️  Creating release $(VERSION)...${RESET}"
	@git tag -a $(VERSION) -m "Release $(VERSION)"
	@git push origin $(VERSION)
	@echo "${GREEN}✅ Release $(VERSION) created and pushed${RESET}"

# Test GitHub Action locally
test-action: build ## 🎬 Test GitHub Action locally
	@echo "${BLUE}🎬 Testing GitHub Action locally...${RESET}"
	@./$(BUILD_DIR)/$(BINARY_NAME) test-content simple test.example.com
	@echo "${GREEN}✅ Action test complete - check output/ directory${RESET}"
