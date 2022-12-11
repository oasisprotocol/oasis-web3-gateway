# Emerald Web3 gateway docker image for local development of Emerald dApps

This folder contains the Dockerfile bundling the following components required to
set up your local development environment for dApps running on Emerald:
- oasis-node and oasis-net-runner
- emerald paratime
- oasis-web3-gateway
- oasis-deposit helper to fund your ethereum wallet

## Prebuilt images

Oasis provides prebuilt `emerald-dev` docker images. To run the `latest` version
based on the `main` branch of `oasis-web3-gateway` repository, run

```sh
docker run -it -p8545:8545 -p8546:8546 oasisprotocol/emerald-dev
```

## Build image locally

To build the docker image, go to your `oasis-web3-gateway` repository root
folder and run:

```sh
make docker
```

To run the compiled image type:

```sh
docker run -it -p8545:8545 -p8546:8546 oasisprotocol/emerald-dev:local
```

## Usage

Once the `emerald-dev` image is up and running, you can connect your hardhat,
truffle or metamask to http://localhost:8545 or ws://localhost:8546.

By default, a random mnemonic will be generated and the first 5 accounts will
be funded 100 ROSE. Flags `-amount`, `-to`, `-n` can be added to specify an
initial ROSE deposit, existing mnemonic and the number of addresses to derive
and fund respectively.

WARNING: The image is running in ephemeral mode. The chain state will be lost
after you quit!
