include common.mk

# Check if Go's linkers flags are set in common.mk and add them as extra flags.
ifneq ($(GOLDFLAGS),)
	GO_EXTRA_FLAGS += -ldflags $(GOLDFLAGS)
endif

all: build

build:
	@$(ECHO) "$(CYAN)*** Building...$(OFF)"
	@$(GO) build $(GOFLAGS) $(GO_EXTRA_FLAGS)
	@$(ECHO) "$(CYAN)*** Everything built successfully!$(OFF)"

test:
	@$(GO) test ./...

# Format code.
fmt:
	@$(ECHO) "$(CYAN)*** Running Go formatters...$(OFF)"
	@gofumpt -s -w .
	@goimports -w -local github.com/oasisprotocol/emerald-web3-gateway .

# Lint code, commits and documentation.
lint-targets := lint-go lint-go-mod-tidy

lint-go:
	@$(ECHO) "$(CYAN)*** Running Go linters...$(OFF)"
	@env -u GOPATH golangci-lint run

lint-go-mod-tidy:
	@$(ECHO) "$(CYAN)*** Checking go mod tidy...$(OFF)"
	@$(ENSURE_GIT_CLEAN)
	@$(CHECK_GO_MOD_TIDY)

lint: $(lint-targets)

# List of targets that are not actual files.
.PHONY: \
	all build \
	test \
	fmt \
	$(lint-targets) lint
