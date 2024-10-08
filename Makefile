include common.mk

# Check if Go's linkers flags are set in common.mk and add them as extra flags.
ifneq ($(GOLDFLAGS),)
	GO_EXTRA_FLAGS += -ldflags $(GOLDFLAGS)
endif

all: build

build:
	@$(ECHO) "$(CYAN)*** Building...$(OFF)"
	@$(MAKE) oasis-web3-gateway
	@$(ECHO) "$(CYAN)*** Everything built successfully!$(OFF)"

oasis-web3-gateway:
	@$(GO) build $(GOFLAGS) $(GO_EXTRA_FLAGS)

clean:
	@$(GO) clean

test:
	@$(GO) test ./...

# Format code.
fmt:
	@$(ECHO) "$(CYAN)*** Running Go formatters...$(OFF)"
	@gofumpt -w .
	@goimports -w -local github.com/oasisprotocol .

# Lint code, commits and documentation.
lint-targets := lint-go lint-go-mod-tidy lint-git

lint-go:
	@$(ECHO) "$(CYAN)*** Running Go linters...$(OFF)"
	@env -u GOPATH golangci-lint run

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

docker: docker-emerald-localnet docker-sapphire-localnet

docker-emerald-localnet:
	@docker build -t ghcr.io/oasisprotocol/emerald-localnet:local --build-arg VERSION=local -f docker/emerald-localnet/Dockerfile .

docker-sapphire-localnet:
	@docker build -t ghcr.io/oasisprotocol/sapphire-localnet:local --build-arg VERSION=local -f docker/sapphire-localnet/Dockerfile .


# List of targets that are not actual files.
.PHONY: \
	all build \
	oasis-web3-gateway \
	clean \
	test \
	fmt \
	$(lint-targets) lint \
	release-build \
	docker
