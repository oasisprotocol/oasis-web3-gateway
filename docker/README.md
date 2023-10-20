# Web3 gateway docker images for local development with bundled Emerald/Sapphire ParaTimes

Subfolders contain Dockerfiles for bundling the following components required to
set up localnet for developing dApps running on Emerald or Sapphire:

- oasis-node and oasis-net-runner
- Emerald or Sapphire ParaTime .orc bundles
- oasis-web3-gateway
- oasis-deposit helper to fund local development accounts

## Prebuilt images

Oasis provides prebuilt `emerald-dev` and `sapphire-dev` docker images. `latest`
versions are based on:
- `stable` branch of `oasis-core`,
- `master` branch of `oasis-sdk`,
- `main` branch of `oasis-web3-gateway` repository.

To use the precompiled images, run:

```sh
docker run -it -p8545:8545 -p8546:8546 ghcr.io/oasisprotocol/emerald-dev # Emerald
docker run -it -p8545:8545 -p8546:8546 ghcr.io/oasisprotocol/sapphire-dev # Sapphire
```

## Build image locally

To build the docker image, go to your `oasis-web3-gateway` repository root
and run:

```sh
make docker
```

To run the compiled image type:

```sh
docker run -it -p8545:8545 -p8546:8546 ghcr.io/oasisprotocol/emerald-dev:local
docker run -it -p8545:8545 -p8546:8546 ghcr.io/oasisprotocol/sapphire-dev:local
```

## Usage

Once the docker image is up and running, you can connect your hardhat,
truffle or metamask to the *Localnet* running at `http://localhost:8545` and
`ws://localhost:8546`.

Chain IDs:
- Emerald Localnet: `0xa514` (`42260`)
- Sapphire Localnet: `0x5afd` (`23293`)

By default, a random mnemonic will be generated and the first 5 accounts will
be funded 10,000 TEST. Flags `-amount`, `-to`, `-n` can be added to specify an
initial ROSE deposit, existing mnemonic and the number of addresses to derive
and fund respectively.

WARNING: The image is running in *ephemeral mode*. A new chain state will be
initialized each time you start the container!
