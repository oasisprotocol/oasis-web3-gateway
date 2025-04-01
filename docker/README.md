# Web3 gateway docker images for local development with bundled Sapphire/Emerald ParaTimes

Subfolders contain Dockerfiles for bundling the following components required to
set up localnet for developing dApps running on Sapphire or Emerald:

- oasis-node and oasis-net-runner
- Sapphire or Emerald ParaTime .orc bundles
- oasis-web3-gateway
- Oasis CLI for funding initial accounts
- Oasis Nexus indexer and Explorer frontend

## Prebuilt images

Oasis provides prebuilt `sapphire-localnet` and `emerald-localnet` docker
images. `latest` versions are based on:
- `stable` branch of `oasis-core`,
- `master` branch of `oasis-sdk`,
- `main` branch of `oasis-web3-gateway` repository.

To use the precompiled images, run:

```sh
docker run -it -p8544-8548:8544-8548 ghcr.io/oasisprotocol/sapphire-localnet # Sapphire
docker run -it -p8544-8548:8544-8548 ghcr.io/oasisprotocol/emerald-localnet # Emerald
```

### Mac M Chips

There is currently no arm64 build available for M Macs, so make sure to force the docker image to use _linux/x86_64_,
like this:

```sh
docker run -it -p8544-8548:8544-8548 --platform linux/x86_64 ghcr.io/oasisprotocol/sapphire-localnet # Sapphire
docker run -it -p8544-8548:8544-8548 --platform linux/x86_64 ghcr.io/oasisprotocol/emerald-localnet # Emerald
```
## Build image locally

To build the docker image, go to your `oasis-web3-gateway` repository root
and run:

```sh
make docker
```

To run the compiled image type:

```sh
docker run -it -p8544-8548:8544-8548 ghcr.io/oasisprotocol/sapphire-localnet:local
docker run -it -p8544-8548:8544-8548 ghcr.io/oasisprotocol/emerald-localnet:local
```

## Usage

Once the docker image is up and running, you can connect your hardhat,
truffle or metamask to the *Localnet* running at `http://localhost:8545` and
`ws://localhost:8546`.

Chain IDs:
- Sapphire Localnet: `0x5afd` (`23293`)
- Emerald Localnet: `0xa514` (`42260`)

By default, a random mnemonic will be generated and the first 5 accounts will
be funded 10,000 TEST. Flags `-amount`, `-to`, `-n` can be added to specify an
initial ROSE deposit, existing mnemonic and the number of addresses to derive
and fund respectively.

By passing `--no-explorer`, the Explorer frontend and Nexus indexer won't be
started (useful if you want to reduce the container startup time a bit,
e.g. for CI tests).

WARNING: The image is running in *ephemeral mode*. A new chain state will be
initialized each time you start the container!

## Debugging

Within the Docker container, the Web3 gateway log messages reside in
`/var/log/oasis-web3-gateway.log`. Logs from the Oasis node instances reside in
the `node.log` files inside `/serverdir/node/net-runner/network` and a
subfolder corresponding to each node's role.

By default, the Oasis Web3 gateway and the Oasis node are configured with the
*warn* verbosity level. To increase verbosity to *debug*, you can run the
Docker container with `-e LOG__LEVEL=debug` for the Web3 gateway and
`-e OASIS_NODE_LOG_LEVEL=debug` for the Oasis node.
