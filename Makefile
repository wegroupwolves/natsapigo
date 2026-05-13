##---------- Preliminaries ----------------------------------------------------
.POSIX:     # Get reliable POSIX behaviour
.SUFFIXES:  # Clear built-in inference rules

##---------- Variables --------------------------------------------------------
PREFIX = /usr/local  # Default installation directory
KO_DOCKER_REPO = sevaho/goforms

##---------- Build targets ----------------------------------------------------

##---------- Export .env as vars ----------------------------------------------
include .env
export

help: ## Show this help message (default)
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: help run css update lint test testr compose deploy serve release

run: ## Run application
	air

sqlgen: ## Generate SQL
	sqlc generate

sqlgenr: ## Generate SQL on repeat
	find db | entr -r make sqlgen

migrate: ## Run migrations
	go run . --migrate

css: ## Run CSS server
	tailwindcss -i ./src/assets/css/app.css -o ./src/assets/static/css/app.css --watch

update: ## Update all dependencies
	go get -u
	go mod tidy

lint: ## Lint
	golangci-lint run --enable-all

test: ## Test
	ginkgo -r

testr: # Test with entr (rerun on file change)
	find . | entr -r ginkgo -r

bench: ## Run benchmarks
	go test ./... -bench=. -benchtime=5s -benchmem 2>&1 | grep -E "Bench|ns/op|PASS|FAIL|ok"
