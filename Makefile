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

# E2E regression testing.
E2E_REGRESSION_SUITES := mainnet_sapphire_2025_09

compress-e2e-regression-caches:
	for suite in $(E2E_REGRESSION_SUITES); do \
		cache_dir="tests/e2e_regression/$$suite/pebble-cache"; \
		archive_path="tests/e2e_regression/$$suite/pebble-cache.tzst"; \
		echo "Compressing $$cache_dir to $$archive_path..."; \
		tar -cf "$$archive_path" --exclude='*.backup' -C "$$(dirname $$cache_dir)" "$$(basename $$cache_dir)" \
			--use-compress-program zstd; \
	done

clean-e2e-regression-caches:
	for suite in $(E2E_REGRESSION_SUITES); do \
		cache_dir="tests/e2e_regression/$$suite/pebble-cache"; \
		archive_path="tests/e2e_regression/$$suite/pebble-cache.tzst"; \
		echo "Removing $$cache_dir and $$archive_path..."; \
		rm -rf "$$cache_dir" "$$archive_path"; \
	done

ensure-e2e-regression-caches:
	for suite in $(E2E_REGRESSION_SUITES); do \
		cache_path="tests/e2e_regression/$$suite/pebble-cache"; \
		archive_path="tests/e2e_regression/$$suite/pebble-cache.tzst"; \
		if [ "$$FORCE" = "1" ]; then \
			echo "Forcing extraction of $$cache_path..."; \
			rm -rf "$$cache_path"; \
		fi; \
		if [ ! -d "$$cache_path" ]; then \
			if [ -f "$$archive_path" ]; then \
				echo "Extracting $$archive_path to $$cache_path..."; \
				mkdir -p "$$(dirname $$cache_path)"; \
				tar -xf "$$archive_path" -C "$$(dirname $$cache_path)" --use-compress-program unzstd; \
			else \
				echo "Archive $$archive_path not found, will run with empty cache."; \
			fi; \
		else \
			echo "Cache already exists for $$suite — skipping."; \
		fi; \
	done

# Run the e2e regression tests, assuming the environment is set up.
test-e2e-regression: oasis-web3-gateway ensure-e2e-regression-caches
	for suite in $(E2E_REGRESSION_SUITES); do ./tests/e2e_regression/run.sh $$suite; done

# Accept the outputs of the e2e regression tests as the new expected outputs.
accept-e2e-regression:
	for suite in $(E2E_REGRESSION_SUITES); do ./tests/e2e_regression/accept.sh $$suite; done

# Check actual vs expected results for e2e regression tests.
check-e2e-regression:
	for suite in $(E2E_REGRESSION_SUITES); do ./tests/e2e_regression/check.sh $$suite; done

# Run dockerized postgres for local development.
postgres:
	@docker ps -a --format '{{.Names}}' | grep -q oasis-web3-gateway-postgres && docker start oasis-web3-gateway-postgres || \
	docker run \
		--name oasis-web3-gateway-postgres \
		-p 5432:5432 \
		-e POSTGRES_USER=postgres \
		-e POSTGRES_PASSWORD=postgres \
		-e POSTGRES_DB=postgres \
		-d postgres -c log_statement=all
	@sleep 3  # Allow time for postgres to start accepting connections.

# Attach to the local DB from "make postgres".
psql:
	@docker exec -it oasis-web3-gateway-postgres psql -U postgres postgres

# Stop and remove the postgres container.
shutdown-postgres:
	@docker rm oasis-web3-gateway-postgres --force

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
	@goreleaser release --clean

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
	compress-e2e-regression-caches clean-e2e-regression-caches ensure-e2e-regression-caches \
	test-e2e-regression accept-e2e-regression check-e2e-regression \
	postgres psql shutdown-postgres \
	fmt \
	$(lint-targets) lint \
	release-build \
	docker
