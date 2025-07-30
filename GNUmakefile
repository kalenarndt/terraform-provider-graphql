GOPATH := $(shell go env | grep GOPATH | sed 's/GOPATH="\(.*\)"/\1/')
PATH := $(GOPATH)/bin:$(PATH)
export $(PATH)
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
TEST_DESTS := $(dir $(wildcard ./e2e/*/*test.tf))

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

fetch: ## download makefile dependencies
	@hash goreleaser 2>/dev/null || go get -u -v github.com/goreleaser/goreleaser

clean: ## cleans previously built binaries and test folders
	@for f in $(TEST_DESTS); do \
	  rm -rf $$f/.terraform; \
	  rm -rf $$f/terraform.d; \
	done
	@rm -rf ./dist;

build: clean fetch ## publishes in dry run mode
	$(GOPATH)/bin/goreleaser --skip-publish --snapshot --skip-sign


.PHONY: test copyplugins

copyplugins: ## copy plugins to test folders
	$(eval COPY_FILES := $(filter %/, $(wildcard ./dist/terraform-provider-graphql*/)))
	$(eval OS_ARCH := $(patsubst ./dist/terraform-provider-graphql_%/, %, $(COPY_FILES)))
	$(eval TEST_FOLDERS := $(foreach p,$(OS_ARCH), $(patsubst %,%terraform.d/plugins/gqlprovidertf.com/examplecorp/graphql/2.0.0/$p,$(TEST_DESTS))))
	@sleep 1
	@mkdir -p $(TEST_FOLDERS);
	@for o in $(OS_ARCH); do \
		for f in $(TEST_DESTS); do \
	    	cp ./dist/terraform-provider-graphql_$$o/* $$f/terraform.d/plugins/gqlprovidertf.com/examplecorp/graphql/2.0.0/$$o; \
		done; \
	done

test: ## test
	@cd e2e && $(MAKE) test

lint: ## run linting and code quality checks
	@./scripts/lint.sh

docs: ## generate documentation
	@./scripts/generate_docs.sh

build-local: ## build provider locally
	@./scripts/build.sh

fulltest: build test ## build and test

quality: lint docs ## run all quality checks

refactor: ## refactor code to use helper functions
	@echo "Refactoring code to use helper functions..."
	@echo "This will move functions from graphql/resource_graphql_mutation.go to helper files"
	@echo "Please review the changes and update imports as needed"

test-helpers: ## test helper functions
	@go test -v ./internal/...