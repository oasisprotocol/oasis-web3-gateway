ARG PARATIME_VERSION=0.9.0-testnet
ARG PARATIME_NAME=sapphire

# Build simple-keymanager, oasis-deposit and, fetch (or optionally build) other components.
FROM ghcr.io/oasisprotocol/oasis-core-dev:stable-24.3.x AS oasis-core-dev

ENV OASIS_CORE_SIMPLE_KM_BRANCH=stable/24.3.x
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
ENV OASIS_CORE_VERSION=24.3
RUN wget https://github.com/oasisprotocol/oasis-core/releases/download/v${OASIS_CORE_VERSION}/oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz \
    && tar -zxvf oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz \
    && strip -S -x oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-node oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-net-runner \
    && mv oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-node oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-net-runner / \
    && rm -rf /oasis_core_${OASIS_CORE_VERSION}_linux_amd64*
#ENV OASIS_CORE_BRANCH=stable/24.3.x
#RUN git clone https://github.com/oasisprotocol/oasis-core.git --branch ${OASIS_CORE_BRANCH} --depth 1 \
#    && cd /oasis-core \
#    && make -C go oasis-node oasis-net-runner \
#    && strip -S -x /oasis-core/go/oasis-net-runner/oasis-net-runner /oasis-core/go/oasis-node/oasis-node \
#    && mv /oasis-core/go/oasis-net-runner/oasis-net-runner /oasis-core/go/oasis-node/oasis-node / \
#    && rm -rf /oasis-core

ENV OASIS_CLI_VERSION=0.10.1
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

# ARG NEXUS_VERSION=0.4.0
# RUN wget https://github.com/oasisprotocol/nexus/releases/download/v${NEXUS_VERSION}/oasis_nexus_${NEXUS_VERSION}_linux_amd64.tar.gz \
#     && tar -zxvf oasis_nexus_${NEXUS_VERSION}_linux_amd64.tar.gz \
#     && strip -S -x /oasis_nexus_${NEXUS_VERSION}_linux_amd64/nexus \
#     && mv /oasis_nexus_${NEXUS_VERSION}_linux_amd64/nexus / \
#     && rm -rf /oasis_nexus_${NEXUS_VERSION}_linux_amd64* && \
#     # We also need to fetch the storage migrations.
#     git clone https://github.com/oasisprotocol/nexus.git nexus-git --branch v${NEXUS_VERSION} --depth 1 \
#     && mv /nexus-git/storage/migrations /nexus-migrations \
#     && rm -rf nexus-git
ARG NEXUS_VERSION=main
RUN git clone https://github.com/oasisprotocol/nexus.git nexus-git --depth 1 \
    && cd nexus-git \
    && git fetch --depth 1 origin ${NEXUS_VERSION} \
    && git checkout ${NEXUS_VERSION} \
    && go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@v1.12 \
    && make codegen-go \
    && go build \
    && strip -S -x /nexus-git/nexus \
    && mv /nexus-git/nexus / \
    && mv /nexus-git/storage/migrations /nexus-migrations \
    && rm -rf /nexus-git

# ARG EXPLORER_REPO=https://github.com/oasisprotocol/explorer.git
# ARG EXPLORER_VERSION=v1.13.0
# TODO: update once some support for localnet is added: https://github.com/oasisprotocol/explorer/issues/1597
ARG EXPLORER_REPO=https://github.com/ptrus/explorer.git
ARG EXPLORER_VERSION=1a78782e10fa266707a428c5fd8ba91966d588a1
COPY docker/common/explorer.env /nexus.env.production
RUN git clone ${EXPLORER_REPO} explorer --depth 1 \
    && cd explorer \
    && git fetch origin ${EXPLORER_VERSION} --depth 1 \
    && git checkout FETCH_HEAD \
    && mv /nexus.env.production .env.production \
    && echo "REACT_APP_LOCALNET_SAPPHIRE=true\nREACT_APP_LOCALNET_EMERALD=false" >> .env.production \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt install -y nodejs \
    && npm install -g yarn \
    && yarn install \
    && NODE_ENV=production yarn build

# Build sapphire-localnet
FROM postgres:16-bookworm
RUN apt update && apt install -y bash libseccomp2 unzip jq binutils curl \
    && su -c "POSTGRES_USER=postgres POSTGRES_PASSWORD=postgres /usr/local/bin/docker-entrypoint.sh postgres &" postgres \
    # Explorer frontend deps.
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt install -y nodejs && npm install -g serve \
    # Install Envoy.
    && curl -fsSL https://apt.envoyproxy.io/signing.key | gpg --dearmor -o /etc/apt/keyrings/envoy-keyring.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/envoy-keyring.gpg] https://apt.envoyproxy.io bookworm main" | tee /etc/apt/sources.list.d/envoy.list \
    && apt update \
    && apt install -y envoy \
    # Clean up.
    && apt clean \
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* \
    && rm -rf /etc/apt/sources.list.d/envoy.list \
    && rm -f /etc/apt/keyrings/envoy-keyring.gpg \
    && rm -rf /root/.npm /root/.node-gyp

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
ENV ENVOY_BINARY=/usr/bin/envoy
ENV ENVOY_CONFIG_FILE=/envoy.yml
ENV OASIS_DEPOSIT_BINARY=/oasis-deposit
ENV OASIS_WEB3_GATEWAY_CONFIG_FILE=/localnet.yml
ENV PARATIME_BINARY=/ronl.elf
ENV KEYMANAGER_BINARY=/simple-keymanager
ENV OASIS_CLI_BINARY=/oasis
ENV OASIS_NEXUS_BINARY=/nexus
ENV OASIS_NEXUS_CONFIG_FILE=/nexus.yml
ENV OASIS_NEXUS_MIGRATIONS=/nexus-migrations
ENV OASIS_EXPLORER_DIR=/explorer
ENV OASIS_UNSAFE_SKIP_AVR_VERIFY=1
ENV OASIS_UNSAFE_ALLOW_DEBUG_ENCLAVES=1
ENV OASIS_UNSAFE_MOCK_SGX=1

ARG VERSION

# Copy binaries.
COPY --from=oasis-core-dev /simple-keymanager ${KEYMANAGER_BINARY}
COPY --from=oasis-core-dev /oasis-deposit ${OASIS_DEPOSIT_BINARY}
COPY --from=oasis-core-dev /oasis-web3-gateway ${OASIS_WEB3_GATEWAY_BINARY}
COPY --from=oasis-core-dev /oasis-node ${OASIS_NODE_BINARY}
COPY --from=oasis-core-dev /oasis-net-runner ${OASIS_NET_RUNNER_BINARY}
COPY --from=oasis-core-dev /oasis ${OASIS_CLI_BINARY}
COPY --from=oasis-core-dev /sapphire-paratime ${PARATIME_BINARY}
COPY --from=oasis-core-dev /nexus ${OASIS_NEXUS_BINARY}
COPY --from=oasis-core-dev /nexus-migrations ${OASIS_NEXUS_MIGRATIONS}
COPY --from=oasis-core-dev /explorer/build ${OASIS_EXPLORER_DIR}

# Create symbolic links for convenience.
RUN ln -s ${OASIS_NODE_BINARY} ${OASIS_CLI_BINARY} /usr/local/bin

COPY docker/common/localnet.yml ${OASIS_WEB3_GATEWAY_CONFIG_FILE}
COPY docker/common/nexus.yml ${OASIS_NEXUS_CONFIG_FILE}
COPY docker/common/envoy.yml ${ENVOY_CONFIG_FILE}
COPY docker/common/start.sh /
COPY docker/common/wait-container-ready.sh /
COPY tests/tools/* /

RUN mkdir /rofls

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

# Explorer port.
EXPOSE 80/tcp
# Envoy proxy port.
EXPOSE 8544/tcp
# Web3 gateway http and ws ports.
EXPOSE 8545/tcp
EXPOSE 8546/tcp
# Nexus port.
EXPOSE 8547/tcp

USER root
ENTRYPOINT ["/start.sh"]
