# oasis-web3-gateway

[![ci-lint](https://github.com/oasisprotocol/oasis-web3-gateway/actions/workflows/ci-lint.yml/badge.svg)](https://github.com/oasisprotocol/oasis-web3-gateway/actions/workflows/ci-lint.yml)
[![ci-test](https://github.com/oasisprotocol/oasis-web3-gateway/actions/workflows/ci-test.yaml/badge.svg)](https://github.com/oasisprotocol/oasis-web3-gateway/actions/workflows/ci-test.yaml)
[![codecov](https://codecov.io/gh/oasisprotocol/oasis-web3-gateway/branch/main/graph/badge.svg?token=WMx1Bg91Hm)](https://codecov.io/gh/oasisprotocol/oasis-web3-gateway)


Web3 Gateway for Oasis-SDK Paratime EVM module.

## Building and Testing

### Prerequisites

- [Go](https://go.dev/) (at least version 1.22).
- [PostgreSQL](https://www.postgresql.org/) (at least version 13.3).

Additionally, for testing:
- [Oasis Core](https://github.com/oasisprotocol/oasis-core) at least version 24.0.x.
- [Emerald Paratime](https://github.com/oasisprotocol/emerald-paratime) at least version 11.x.x.
- (or) [Sapphire Paratime](https://github.com/oasisprotocol/sapphire-paratime) at least version 0.7.3.

### Build

To build the binary run:

```bash
make build
```

### Testing on Localnet

The Docker containers can be used to test changes to the gateway or run tests by
bind-mounting the runtime state directory (`/serverdir/node`) into your local
filesystem.

```bash
docker run --rm -ti -p5432:5432 -p8545:8545 -p8546:8546 -v /tmp/eth-runtime-test:/serverdir/node ghcr.io/oasisprotocol/sapphire-localnet:local -test-mnemonic -n 4
```

If needed, the `oasis-web3-gateway` or `sapphire-paratime` executables could also be
bind-mounted into the container, allowing for quick turnaround time when testing
the full gateway & paratime stack together.

### Running tests

For running tests, start the docker network without the gateway, by setting the `OASIS_DOCKER_NO_GATEWAY=yes` environment variable:

```bash
docker run --rm -ti -e OASIS_DOCKER_NO_GATEWAY=yes -p5432:5432 -p8544-8546:8544-8546 -v /tmp/eth-runtime-test:/serverdir/node ghcr.io/oasisprotocol/sapphire-localnet:local -test-mnemonic -n 4
```

Once bootstrapped, run the tests:

```bash
make test
```

[oasis-core]: https://github.com/oasisprotocol/oasis-core

## Localnet Docker images

You can also build `emerald-dev` and `sapphire-dev` docker images with a
complete confidential and non-confidential Localnet Oasis stack respectively
for development, CI and testing.

```bash
make docker
```

Check out [docker folder] for more information.

[docker folder]: docker/README.md

## Running the Gateway on Testnet/Mainnet

The gateway connects to an Emerald/Sapphire enabled [Oasis ParaTime Client Node].

In addition to the general instructions for setting up an Oasis ParaTime Client
Node update the node configuration (e.g. `config.yml`) as follows:

```yaml
# ... sections not relevant are omitted ...
runtime:
  mode: client
  paths:
    - <orc_bundle_path>

  config:
    "<paratime_id>":
      # The following allows the gateway to perform gas estimation of smart
      # contract calls (not allowed by default).
      estimate_gas_by_simulating_contracts: true
      # The following allows some more resource-intensive queries to be called
      # by the gateway (not allowed by default).
      allowed_queries:
        - all_expensive: true
```

Set up the config file (e.g. `gateway.yml`) appropriately:

```yaml
runtime_id: <paratime_id>
node_address: "unix:<path-to-oasis-node-unix-socket>"
enable_pruning: false
pruning_step: 100000
indexing_start: 0

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
  chain_id: <chain_id>
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
  oasis_rpcs: true # Enable Oasis-specific requests for confidentiality etc.
```

Note: all configuration settings can also be set via environment variables. For example to set the database password use:

```bash
DATABASE__PASSWORD: <postgresql_password>
```

environment variable.

Start the gateway by running the `oasis-web3-gateway` binary:

```bash
oasis-web3-gateway --config gateway.yml
```

[Oasis ParaTime Client Node]: https://docs.oasis.io/node/run-your-node/paratime-client-node

### Wipe state to force a complete reindex

To wipe the DB state and force a reindexing use the `truncate-db` subcommand:

```bash
oasis-web3-gateway truncate-db --config gateway.yml --unsafe
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
