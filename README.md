# emerald-web3-gateway

[![ci-lint](https://github.com/oasisprotocol/emerald-web3-gateway/actions/workflows/ci-lint.yml/badge.svg)](https://github.com/oasisprotocol/emerald-web3-gateway/actions/workflows/ci-lint.yml)
[![ci-test](https://github.com/oasisprotocol/emerald-web3-gateway/actions/workflows/ci-test.yaml/badge.svg)](https://github.com/oasisprotocol/emerald-web3-gateway/actions/workflows/ci-test.yaml)
[![codecov](https://codecov.io/gh/oasisprotocol/emerald-web3-gateway/branch/main/graph/badge.svg?token=WMx1Bg91Hm)](https://codecov.io/gh/oasisprotocol/emerald-web3-gateway)


Web3 Gateway for Oasis Emerald EVM.

## Building and Testing

### Prerequisites

- [Go](https://go.dev/) (at least version 1.17.3).
- [PostgreSQL](https://www.postgresql.org/) (at least version 13.3).

Additionally, for testing:
- [Oasis Core](https://github.com/oasisprotocol/oasis-core) version 21.3.7.
- [Emerald Paratime](https://github.com/oasisprotocol/emerald-paratime) version 6.0.0.

### Build

To build the binary run:

```bash
make build
```

### Test

To run tests:

Start PostgreSQL (for testing [Postgres Docker](https://hub.docker.com/_/postgres) container can be used):

```bash
docker run --rm --name postgres -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:13.3-alpine
```

In a separate terminal, start an Oasis development network:

```bash
export OASIS_EMERALD_VERSION=6.0.0
export OASIS_NET_RUNNER=<path-to-oasis-core-artifacts>/oasis-net-runner
export OASIS_NODE=<path-to-oasis-core-artifacts>/oasis-node
export OASIS_EMERALD_PARATIME=<path-to-emerald-paratime>/emerald-paratime
export OASIS_NODE_DATADIR=/tmp/oasis-evm-gateway-tests

./tests/tools/spinup-oasis-stack.sh
```

Run tests:

```bash
make test
```

## Running the Gateway on Testnet/Mainnet

The gateway connects to an Emerald enabled [Oasis Paratime Client Node](https://docs.oasis.dev/general/run-a-node/set-up-your-node/run-a-paratime-client-node).

Set up the config file (e.g. `gateway.yml`) appropriately:

```yaml
runtime_id: <emerald_paratime_id>
node_address: "unix:<path-to-oasis-node-unix-socket>"
enable_pruning: false
pruning_step: 100000

log:
  level: debug
  format: json

database:
  host: <postgresql_host>
  port: <postgresql_port>
  db: <postgresql_db>
  user: <postgresql_user>
  password: <postgresql_password>
  dial_timeout: 5
  read_timeout: 10
  write_timeout: 5
  max_open_conns: 0

gateway:
  chain_id: <emerald_chain_id>
  http:
    host: <gateway_listen_interface>
    port: <gateway_listen_port>
    cors: ["*"]
  ws:
    host: <gateway_listen_interface>
    port: <gateway_listen_websocket_port>
    origins: ["*"]
  method_limits:
    get_logs_max_rounds: 100
```

Note: all configuration settings can also be set via environment variables. For example to set the database password use:

```bash
DATABASE__PASSWORD: <postgresql_password>
```

environment variable.

Start the gateway by running the `emerald-web3-gateway` binary:

```bash
emerald-web3-gateway --config gateway.yml
```

### Wipe state to force a complete reindex

To wipe the DB state and force a reindexing use the `truncate-db` subcommand:

```bash
emerald-web3-gateway truncate-db --config gateway.yml --unsafe
```

**Warning: this will wipe all existing state in the Postgres DB and can
lead to extended downtime while the Web3 Gateway is reindexing the blocks.**

## Contributing

See our [Contributing Guidelines](CONTRIBUTING.md).

### Versioning

See our [Versioning] document.

[Versioning]: docs/versioning.md

### Release Process

See our [Release Process] document.

[Release Process]: docs/release-process.md

## Credits

Parts of the code heavily based on [go-ethereum](https://github.com/ethereum/go-ethereum).
