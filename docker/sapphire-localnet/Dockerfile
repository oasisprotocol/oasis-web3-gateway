# Build oasis-web3-gateway
FROM golang:1.22.3 AS oasis-web3-gateway

COPY . /go/oasis-web3-gateway
RUN cd oasis-web3-gateway && make && strip -S -x oasis-web3-gateway docker/common/oasis-deposit/oasis-deposit

# Build simple-keymanager
FROM ghcr.io/oasisprotocol/oasis-core-dev:stable-24.2.x AS oasis-core-dev

ENV OASIS_UNSAFE_MOCK_SGX=1
ENV OASIS_UNSAFE_ALLOW_DEBUG_ENCLAVES=1
ENV OASIS_CORE_VERSION=stable/24.2.x
ENV OASIS_CLI_VERSION=0.10.0

RUN git clone https://github.com/oasisprotocol/oasis-core.git --branch ${OASIS_CORE_VERSION} --depth 1 \
	&& cd oasis-core/go/ \
	&& make oasis-node oasis-net-runner \
	&& cd ../../ \
	&& cd oasis-core/tests/runtimes/simple-keymanager \
	&& CARGO_PROFILE_RELEASE_LTO=true CARGO_PROFILE_RELEASE_OPT_LEVEL=s CARGO_PROFILE_RELEASE_CODEGEN_UNITS=1 OASIS_UNSAFE_SKIP_AVR_VERIFY=1 cargo build --release --features debug-mock-sgx \
	&& cd ../../../../ \
    && cd oasis-core \
    && cargo build -p oasis-core-runtime-loader --release \
    && cd .. \
	&& git clone https://github.com/oasisprotocol/cli.git --branch v${OASIS_CLI_VERSION} --depth 1 \
	&& cd cli \
	&& make CGO_ENABLED=0

# Build sapphire-localnet
FROM postgres:16-alpine
RUN apk add --no-cache bash gcompat libseccomp jq binutils curl \
	&& su -c "POSTGRES_USER=postgres POSTGRES_PASSWORD=postgres /usr/local/bin/docker-entrypoint.sh postgres &" postgres

# Docker-specific variables
ENV PARATIME_VERSION=0.8.1-testnet
ENV PARATIME_NAME=sapphire
ENV GATEWAY__CHAIN_ID=0x5afd

# start.sh and spinup-oasis-stack.sh ENV variables.
ENV OASIS_NODE_LOG_LEVEL=warn
ENV OASIS_NODE=/oasis-node
ENV OASIS_NET_RUNNER=/oasis-net-runner
ENV OASIS_NODE_DATADIR=/serverdir/node
ENV OASIS_WEB3_GATEWAY=/oasis-web3-gateway
ENV OASIS_DEPOSIT=/oasis-deposit
ENV OASIS_WEB3_GATEWAY_CONFIG_FILE=/localnet.yml
ENV PARATIME=/ronl.elf
ENV KEYMANAGER_BINARY=/simple-keymanager
ENV CLI_BINARY=/oasis
ENV OASIS_UNSAFE_SKIP_AVR_VERIFY=1
ENV OASIS_UNSAFE_ALLOW_DEBUG_ENCLAVES=1
ENV OASIS_UNSAFE_MOCK_SGX=1

ARG VERSION

# oasis-node and oasis-net-runner
COPY --from=oasis-core-dev /oasis-core/go/oasis-node/oasis-node ${OASIS_NODE}
COPY --from=oasis-core-dev /oasis-core/go/oasis-net-runner/oasis-net-runner ${OASIS_NET_RUNNER}

# simple-keymanager
COPY --from=oasis-core-dev /oasis-core/target/release/simple-keymanager ${KEYMANAGER_BINARY}

# oasis-core-runtime-loader
COPY --from=oasis-core-dev /oasis-core/target/release/oasis-core-runtime-loader ${OASIS_CORE_RUNTIME_LOADER}

# cli
COPY --from=oasis-core-dev /cli/oasis ${CLI_BINARY}

# oasis-web3-gateway binary, config, spinup-* scripts and staking_genesis.json.
COPY --from=oasis-web3-gateway /go/oasis-web3-gateway/oasis-web3-gateway ${OASIS_WEB3_GATEWAY}
COPY --from=oasis-web3-gateway /go/oasis-web3-gateway/docker/common/oasis-deposit/oasis-deposit ${OASIS_DEPOSIT}
COPY docker/common/localnet.yml ${OASIS_WEB3_GATEWAY_CONFIG_FILE}
COPY docker/common/start.sh /
COPY tests/tools/* /

# Configure paratime.
# XXX: Build locally.
COPY docker/sapphire-localnet/sapphire-paratime ${PARATIME}
RUN echo "Configure the ParaTime." \
#	&& mkdir -p "$(dirname ${PARATIME})" \
#   && wget --quiet "https://github.com/oasisprotocol/${PARATIME_NAME}-paratime/releases/download/v${PARATIME_VERSION}/localnet-${PARATIME_NAME}-paratime.orc" -O "/paratime.orc" \
#   && unzip "paratime.orc" \
#   && chmod a+x "runtime.elf" \
#	&& rm "paratime.orc" \
#	&& echo "" \
	&& echo "Write VERSION information." \
	&& echo "${VERSION}" > /VERSION \
	&& strip -S -x ${OASIS_NET_RUNNER} ${OASIS_NODE} ${KEYMANAGER_BINARY} ${CLI_BINARY} \
	&& echo "" \
	&& ls -l / \
	&& echo "" \
	&& echo "*** Oasis Node:" \
	&& ${OASIS_NODE} --version \
	&& echo "" \
	&& echo "*** Oasis CLI:" \
	&& ${CLI_BINARY} --version \
	&& echo ""

# Web3 gateway http and ws ports.
EXPOSE 8545/tcp
EXPOSE 8546/tcp

USER root
ENTRYPOINT ["/start.sh"]
