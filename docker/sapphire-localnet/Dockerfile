ARG PARATIME_VERSION=0.8.2
ARG PARATIME_NAME=sapphire

# Build simple-keymanager, oasis-deposit and, fetch (or optionally build) other components.
FROM ghcr.io/oasisprotocol/oasis-core-dev:stable-24.2.x AS oasis-core-dev

ENV OASIS_CORE_SIMPLE_KM_BRANCH=stable/24.2.x
RUN git clone https://github.com/oasisprotocol/oasis-core.git oasis-core-km --branch ${OASIS_CORE_SIMPLE_KM_BRANCH} --depth 1 \
    && cd /oasis-core-km/tests/runtimes/simple-keymanager \
    && OASIS_UNSAFE_ALLOW_DEBUG_ENCLAVES=1 CARGO_PROFILE_RELEASE_LTO=true CARGO_PROFILE_RELEASE_OPT_LEVEL=s CARGO_PROFILE_RELEASE_CODEGEN_UNITS=1 OASIS_UNSAFE_SKIP_AVR_VERIFY=1 \
    cargo build --release --features debug-mock-sgx \
    && strip -S -x /oasis-core-km/target/release/simple-keymanager \
    && mv /oasis-core-km/target/release/simple-keymanager / \
    && rm -rf /oasis-core-km

COPY docker/common/oasis-deposit /oasis-deposit-git
RUN cd oasis-deposit-git \
    && go build \
    && strip -S -x /oasis-deposit-git/oasis-deposit \
    && mv /oasis-deposit-git/oasis-deposit / \
    && rm -rf /oasis-deposit-git

# Fetch latest released components. Replace with commented snippet below to build manually.
ENV OASIS_CORE_VERSION=24.2
RUN wget https://github.com/oasisprotocol/oasis-core/releases/download/v${OASIS_CORE_VERSION}/oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz \
    && tar -zxvf oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz \
    && strip -S -x oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-node oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-net-runner \
    && mv oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-node oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-net-runner / \
    && rm -rf /oasis_core_${OASIS_CORE_VERSION}_linux_amd64*
#ENV OASIS_CORE_BRANCH=stable/24.2.x
#RUN git clone https://github.com/oasisprotocol/oasis-core.git --branch ${OASIS_CORE_BRANCH} --depth 1 \
#    && cd /oasis-core \
#    && make -C go oasis-node oasis-net-runner \
#    && strip -S -x /oasis-core/go/oasis-net-runner/oasis-net-runner /oasis-core/go/oasis-node/oasis-node \
#    && mv /oasis-core/go/oasis-net-runner/oasis-net-runner /oasis-core/go/oasis-node/oasis-node / \
#    && rm -rf /oasis-core

ENV OASIS_CLI_VERSION=0.10.0
RUN wget https://github.com/oasisprotocol/cli/releases/download/v${OASIS_CLI_VERSION}/oasis_cli_${OASIS_CLI_VERSION}_linux_amd64.tar.gz \
    && tar -zxvf oasis_cli_${OASIS_CLI_VERSION}_linux_amd64.tar.gz \
    && strip -S -x /oasis_cli_${OASIS_CLI_VERSION}_linux_amd64/oasis \
    && mv /oasis_cli_${OASIS_CLI_VERSION}_linux_amd64/oasis / \
    && rm -rf /oasis_cli_${OASIS_CLI_VERSION}_linux_amd64*
#ENV OASIS_CLI_BRANCH=master
#RUN git clone https://github.com/oasisprotocol/cli.git --branch ${OASIS_CLI_BRANCH} --depth 1 \
#    && make -C cli CGO_ENABLED=0 \
#    && strip -S -x /cli/oasis
#    && mv /cli/oasis / \
#    && rm -rf cli

ENV OASIS_WEB3_GATEWAY_VERSION=5.1.0
RUN wget https://github.com/oasisprotocol/oasis-web3-gateway/releases/download/v${OASIS_WEB3_GATEWAY_VERSION}/oasis_web3_gateway_${OASIS_WEB3_GATEWAY_VERSION}_linux_amd64.tar.gz \
    && tar -zxvf oasis_web3_gateway_${OASIS_WEB3_GATEWAY_VERSION}_linux_amd64.tar.gz \
    && strip -S -x /oasis_web3_gateway_${OASIS_WEB3_GATEWAY_VERSION}_linux_amd64/oasis-web3-gateway \
    && mv /oasis_web3_gateway_${OASIS_WEB3_GATEWAY_VERSION}_linux_amd64/oasis-web3-gateway / \
    && rm -rf /oasis_web3_gateway_${OASIS_WEB3_GATEWAY_VERSION}_linux_amd64*
#ENV OASIS_WEB3_GATEWAY_BRANCH=main
#RUN git clone https://github.com/oasisprotocol/oasis-web3-gateway.git oasis-web3-gateway-git --branch ${OASIS_WEB3_GATEWAY_BRANCH} --depth 1 \
#    && make -C oasis-web3-gateway-git \
#    && strip -S -x /oasis-web3-gateway-git/oasis-web3-gateway \
#    && mv /oasis-web3-gateway-git/oasis-web3-gateway / \
#    && rm -rf /oasis-web3-gateway-git

ARG PARATIME_VERSION
ARG PARATIME_NAME
RUN wget https://github.com/oasisprotocol/${PARATIME_NAME}-paratime/releases/download/v${PARATIME_VERSION}/localnet-${PARATIME_NAME}-paratime.orc \
    && unzip "localnet-${PARATIME_NAME}-paratime.orc" "ronl.elf" \
    && chmod a+x "ronl.elf" \
    && mv "ronl.elf" "sapphire-paratime" \
    && strip -S -x "sapphire-paratime" \
    && rm "localnet-${PARATIME_NAME}-paratime.orc"
#ENV OASIS_SAPPHIRE_PARATIME_BRANCH=main
#RUN git clone https://github.com/oasisprotocol/sapphire-paratime.git sapphire-paratime-git --branch ${OASIS_SAPPHIRE_PARATIME_BRANCH} --depth 1 \
#    && cd sapphire-paratime-git/runtime \
#    && OASIS_UNSAFE_SKIP_AVR_VERIFY=1 OASIS_UNSAFE_ALLOW_DEBUG_ENCLAVES=1 OASIS_UNSAFE_USE_LOCALNET_CHAINID=1 \
#    cargo build --release --locked --features debug-mock-sgx \
#    && strip -S -x /sapphire-paratime-git/runtime/target/release/sapphire-paratime \
#    && mv /sapphire-paratime-git/runtime/target/release/sapphire-paratime / \
#    && rm -rf sapphire-paratime-git

# Build sapphire-localnet
FROM postgres:16-alpine
RUN apk add --no-cache bash gcompat libseccomp jq binutils curl \
    && su -c "POSTGRES_USER=postgres POSTGRES_PASSWORD=postgres /usr/local/bin/docker-entrypoint.sh postgres &" postgres

# Docker-specific variables
ARG PARATIME_VERSION
ARG PARATIME_NAME
ENV PARATIME_VERSION=${PARATIME_VERSION}
ENV PARATIME_NAME=${PARATIME_NAME}
ENV GATEWAY__CHAIN_ID=0x5afd

# start.sh and spinup-oasis-stack.sh ENV variables.
ENV OASIS_NODE_LOG_LEVEL=warn
ENV OASIS_NODE_BINARY=/oasis-node
ENV OASIS_NET_RUNNER_BINARY=/oasis-net-runner
ENV OASIS_NODE_DATADIR=/serverdir/node
ENV OASIS_WEB3_GATEWAY_BINARY=/oasis-web3-gateway
ENV OASIS_DEPOSIT_BINARY=/oasis-deposit
ENV OASIS_WEB3_GATEWAY_CONFIG_FILE=/localnet.yml
ENV PARATIME_BINARY=/ronl.elf
ENV KEYMANAGER_BINARY=/simple-keymanager
ENV OASIS_CLI_BINARY=/oasis
ENV OASIS_UNSAFE_SKIP_AVR_VERIFY=1
ENV OASIS_UNSAFE_ALLOW_DEBUG_ENCLAVES=1
ENV OASIS_UNSAFE_MOCK_SGX=1

ARG VERSION

# Copy local builds.
COPY --from=oasis-core-dev /simple-keymanager ${KEYMANAGER_BINARY}
COPY --from=oasis-core-dev /oasis-deposit ${OASIS_DEPOSIT_BINARY}
COPY --from=oasis-core-dev /oasis-web3-gateway ${OASIS_WEB3_GATEWAY_BINARY}
COPY --from=oasis-core-dev /oasis-node ${OASIS_NODE_BINARY}
COPY --from=oasis-core-dev /oasis-net-runner ${OASIS_NET_RUNNER_BINARY}
COPY --from=oasis-core-dev /oasis ${OASIS_CLI_BINARY}
COPY --from=oasis-core-dev /sapphire-paratime ${PARATIME_BINARY}

COPY docker/common/localnet.yml ${OASIS_WEB3_GATEWAY_CONFIG_FILE}
COPY docker/common/start.sh /
COPY docker/common/wait-container-ready.sh /
COPY tests/tools/* /

# Configure paratime.
RUN echo "Write VERSION information." \
    && echo "${VERSION}" > /VERSION \
    && echo \
    && ls -l / \
    && echo \
    && echo "*** Oasis Node:" \
    && ${OASIS_NODE_BINARY} --version \
    && echo \
    && echo "*** Oasis CLI:" \
    && ${OASIS_CLI_BINARY} --version \
    && echo

# Web3 gateway http and ws ports.
EXPOSE 8545/tcp
EXPOSE 8546/tcp

USER root
ENTRYPOINT ["/start.sh"]
