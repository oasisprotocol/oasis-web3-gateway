include common.mk

# Check if Go's linkers flags are set in common.mk and add them as extra flags.
ifneq ($(GOLDFLAGS),)
	GO_EXTRA_FLAGS += -ldflags $(GOLDFLAGS)
endif

all: build

build:
	@$(ECHO) "$(CYAN)*** Building...$(OFF)"
	@$(MAKE) oasis-web3-gateway
	@$(MAKE) docker/common/oasis-deposit/oasis-deposit
	@$(ECHO) "$(CYAN)*** Everything built successfully!$(OFF)"

oasis-web3-gateway:
	@$(GO) build $(GOFLAGS) $(GO_EXTRA_FLAGS)

docker/common/oasis-deposit/oasis-deposit:
	@cd docker/common/oasis-deposit && $(GO) build

clean:
	@$(GO) clean
	@cd docker/common/oasis-deposit && $(GO) clean

test:
	@$(GO) test ./...

# Format code.
fmt:
	@$(ECHO) "$(CYAN)*** Running Go formatters...$(OFF)"
	@gofumpt -w .
	@goimports -w -local github.com/oasisprotocol/oasis-web3-gateway .

# Lint code, commits and documentation.
lint-targets := lint-go lint-go-mod-tidy lint-git

lint-go:
	@$(ECHO) "$(CYAN)*** Running Go linters...$(OFF)"
	@env -u GOPATH golangci-lint run
	@cd docker/common/oasis-deposit && env -u GOPATH golangci-lint run

lint-go-mod-tidy:
	@$(ECHO) "$(CYAN)*** Checking go mod tidy...$(OFF)"
	@$(ENSURE_GIT_CLEAN)
	@$(CHECK_GO_MOD_TIDY)

lint-git:
	@$(CHECK_GITLINT) || \
	($(ECHO) "See commit style guide at: https://github.com/oasisprotocol/oasis-web3-gateway/blob/main/CONTRIBUTING.md#git-commit-messages" && \
	exit 1)

lint: $(lint-targets)

release-build:
	@goreleaser release --rm-dist

docker: docker-emerald-dev docker-sapphire-dev

docker-emerald-dev:
	@docker build -t ghcr.io/oasisprotocol/emerald-dev:local --build-arg VERSION=local -f docker/emerald-dev/Dockerfile .

docker-sapphire-dev:
	@docker build -t ghcr.io/oasisprotocol/sapphire-dev:local --build-arg VERSION=local -f docker/sapphire-dev/Dockerfile .


# List of targets that are not actual files.
.PHONY: \
	all build \
	oasis-web3-gateway \
	docker/common/oasis-deposit/oasis-deposit \
	clean \
	test \
	fmt \
	$(lint-targets) lint \
	release-build \
	docker
