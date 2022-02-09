# Emerald Web3 gateway docker image for local development of Emerald dApps

This folder contains the Dockerfile bundling the following components required to
set up your local development environment for dApps running on Emerald:
- oasis-node and oasis-net-runner
- emerald paratime
- emerald-web3-gateway
- oasis-deposit helper to fund your ethereum wallet

To build the docker image, go to your emerald-web3-gateway root folder
and run:

```sh
make docker
```

To run the Emerald development environment locally run:

```sh
docker run -it -p8545:8545 -p8546:8546 emerald-dev
```

You can now connect with your hardhat, truffle or metamask to
http://localhost:8545 or ws://localhost:8546

By default, a random mnemonic will be generated and the first 5 accounts will
be funded 100 ROSE. Flags `-amount`, `-to`, `-n` can be added to specify an
initial ROSE deposit, existing mnemonic and the number of addresses to derive
and fund respectively.

WARNING: The image is running in ephemeral mode. The chain state will be lost
after you quit!
