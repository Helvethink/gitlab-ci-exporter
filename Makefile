##################################################
# Gitlab-CI Exporter - Makefile
##################################################
NAME           := gitlab-ci-exporter
FILES          := $(shell git ls-files */*.go)
COVERAGE_FILE  := coverage.out
REPOSITORY     := "helvethink/$(NAME)"
.DEFAULT_GOAL  := help
GOLANG_VERSION := 1.23
GOLANGCI_LINT_VERSION ?= v2.1.5

.PHONY: lint
lint: ## Run all lint related tests againts the codebase
	@echo "--- Linting code..."
	docker run --rm -w /workdir -v $(PWD):/workdir golangci/golangci-lint:$(GOLANGCI_LINT_VERSION) golangci-lint run -c .golangci.yml --fix

.PHONY: run
run: ## Run the tests against the codebase
	@echo "Run app:"
	$(GO) run cmd/$(APP_NAME)/main.go

.PHONY: coverage
coverage: ## Prints coverage report
	command

.PHONY: coverage-html
coverage-html: ## Generates coverage report and displays it in the browser
	go tool cover -html=coverage.out

.PHONY: install
install: ## Build and install locally the binary (dev purpose)
	go install ./cmd/$(NAME)

.PHONY: build
build: ## Build the binaries using local GOOS
	go build -o ./bin/$(NAME) ./cmd/$(NAME)

.PHONY: clean
clean: ## Remove binary if it exists
	rm -f $(NAME)

.PHONY: all
all: lint run build coverage ## Test, builds and ship package for all supported platforms

.PHONY: help
help: ## Displays this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
