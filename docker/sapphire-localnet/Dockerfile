ARG PARATIME_VERSION=0.9.1-testnet
ARG PARATIME_NAME=sapphire

FROM ghcr.io/oasisprotocol/oasis-core-dev:stable-24.3.x AS oasis-core-dev

ARG PARATIME_VERSION
ARG PARATIME_NAME

# If you uncomment the _BRANCH variant, that component will be freshly built
# from that respective git branch instead of using the pre-packaged release
# variant.
# Note that the simple keymanager is always freshly built because we need to
# specify different build options from what is in the release version.
ARG OASIS_CORE_SIMPLE_KM_BRANCH=stable/24.3.x
ARG OASIS_CORE_VERSION=24.3.1
#ARG OASIS_CORE_BRANCH=stable/24.3.x
ARG OASIS_CLI_VERSION=0.10.3
#ARG OASIS_CLI_BRANCH=master
ARG OASIS_WEB3_GATEWAY_BRANCH=v5.2.0-rc1
#ARG OASIS_WEB3_GATEWAY_VERSION=main
#ARG OASIS_SAPPHIRE_PARATIME_BRANCH=main

ARG NEXUS_VERSION=0.5.3
#ARG NEXUS_BRANCH=main

ARG EXPLORER_BRANCH=v1.16.1

COPY docker/common /out
COPY tests/tools/* /out/

RUN set -e; \
    git clone https://github.com/oasisprotocol/oasis-core.git oasis-core-km --branch ${OASIS_CORE_SIMPLE_KM_BRANCH} --depth 1 \
    && cd /oasis-core-km/tests/runtimes/simple-keymanager \
    && OASIS_UNSAFE_ALLOW_DEBUG_ENCLAVES=1 CARGO_PROFILE_RELEASE_LTO=true CARGO_PROFILE_RELEASE_OPT_LEVEL=s CARGO_PROFILE_RELEASE_CODEGEN_UNITS=1 OASIS_UNSAFE_SKIP_AVR_VERIFY=1 \
    cargo build --release --features debug-mock-sgx \
    && strip -s /oasis-core-km/target/release/simple-keymanager \
    && mv /oasis-core-km/target/release/simple-keymanager /out \
    && rm -rf /oasis-core-km \
    \
    && cd / \
    && if [ "x${OASIS_CORE_BRANCH}" != "x" ]; then \
        git clone https://github.com/oasisprotocol/oasis-core.git --depth 1 \
        && cd /oasis-core \
        && git fetch origin ${OASIS_CORE_BRANCH} --depth 1 \
        && git checkout FETCH_HEAD \
        && make -C go oasis-node oasis-net-runner \
        && strip -s /oasis-core/go/oasis-net-runner/oasis-net-runner /oasis-core/go/oasis-node/oasis-node \
        && mv /oasis-core/go/oasis-net-runner/oasis-net-runner /oasis-core/go/oasis-node/oasis-node /out \
        && rm -rf /oasis-core; \
    else \
        wget https://github.com/oasisprotocol/oasis-core/releases/download/v${OASIS_CORE_VERSION}/oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz \
        && tar -zxvf oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz \
        && strip -s oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-node oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-net-runner \
        && mv oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-node oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-net-runner /out \
        && rm -rf /oasis_core_${OASIS_CORE_VERSION}_linux_amd64*; \
    fi \
    \
    && cd / \
    && if [ "x${OASIS_CLI_BRANCH}" != "x" ]; then \
        git clone https://github.com/oasisprotocol/cli.git --depth 1 \
        && cd /cli \
        && git fetch origin ${OASIS_CLI_BRANCH} --depth 1 \
        && git checkout FETCH_HEAD \
        && make CGO_ENABLED=0 \
        && strip -s oasis \
        && mv oasis /out \
        && rm -rf /cli; \
    else \
        wget https://github.com/oasisprotocol/cli/releases/download/v${OASIS_CLI_VERSION}/oasis_cli_${OASIS_CLI_VERSION}_linux_amd64.tar.gz \
        && tar -zxvf oasis_cli_${OASIS_CLI_VERSION}_linux_amd64.tar.gz \
        && strip -s /oasis_cli_${OASIS_CLI_VERSION}_linux_amd64/oasis \
        && mv /oasis_cli_${OASIS_CLI_VERSION}_linux_amd64/oasis /out \
        && rm -rf /oasis_cli_${OASIS_CLI_VERSION}_linux_amd64*; \
    fi \
    \
    && cd / \
    && if [ "x${OASIS_WEB3_GATEWAY_BRANCH}" != "x" ]; then \
        git clone https://github.com/oasisprotocol/oasis-web3-gateway.git oasis-web3-gateway-git --depth 1 \
        && cd /oasis-web3-gateway-git \
        && git fetch origin ${OASIS_WEB3_GATEWAY_BRANCH} --depth 1 \
        && git checkout FETCH_HEAD \
        && make \
        && strip -s oasis-web3-gateway \
        && mv oasis-web3-gateway /out \
        && rm -rf /oasis-web3-gateway-git; \
    else \
        wget https://github.com/oasisprotocol/oasis-web3-gateway/releases/download/v${OASIS_WEB3_GATEWAY_VERSION}/oasis_web3_gateway_${OASIS_WEB3_GATEWAY_VERSION}_linux_amd64.tar.gz \
        && tar -zxvf oasis_web3_gateway_${OASIS_WEB3_GATEWAY_VERSION}_linux_amd64.tar.gz \
        && strip -s /oasis_web3_gateway_${OASIS_WEB3_GATEWAY_VERSION}_linux_amd64/oasis-web3-gateway \
        && mv /oasis_web3_gateway_${OASIS_WEB3_GATEWAY_VERSION}_linux_amd64/oasis-web3-gateway /out \
        && rm -rf /oasis_web3_gateway_${OASIS_WEB3_GATEWAY_VERSION}_linux_amd64*; \
    fi \
    \
    && cd / \
    && if [ "x${OASIS_SAPPHIRE_PARATIME_BRANCH}" != "x" ]; then \
        git clone https://github.com/oasisprotocol/sapphire-paratime.git sapphire-paratime-git --depth 1 \
        && cd /sapphire-paratime-git \
        && git fetch origin ${OASIS_SAPPHIRE_PARATIME_BRANCH} --depth 1 \
        && git checkout FETCH_HEAD \
        && cd runtime \
        && OASIS_UNSAFE_SKIP_AVR_VERIFY=1 OASIS_UNSAFE_ALLOW_DEBUG_ENCLAVES=1 OASIS_UNSAFE_USE_LOCALNET_CHAINID=1 \
        cargo build --release --locked --features debug-mock-sgx \
        && strip -s /sapphire-paratime-git/runtime/target/release/sapphire-paratime \
        && mv /sapphire-paratime-git/runtime/target/release/sapphire-paratime /out/ronl.elf \
        && rm -rf sapphire-paratime-git; \
    else \
        wget https://github.com/oasisprotocol/${PARATIME_NAME}-paratime/releases/download/v${PARATIME_VERSION}/localnet-${PARATIME_NAME}-paratime.orc \
        && unzip "localnet-${PARATIME_NAME}-paratime.orc" ronl.elf \
        && chmod a+x ronl.elf \
        && strip -s ronl.elf \
        && mv ronl.elf /out \
        && rm "localnet-${PARATIME_NAME}-paratime.orc"; \
    fi \
    \
    && cd / \
    && if [ "x${NEXUS_BRANCH}" != "x" ]; then \
        git clone https://github.com/oasisprotocol/nexus.git nexus-git --depth 1 \
        && cd nexus-git \
        && git fetch origin ${NEXUS_BRANCH} --depth 1 \
        && git checkout FETCH_HEAD \
        && go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@v1.12 \
        && make codegen-go \
        && go build \
        && strip -s /nexus-git/nexus \
        && mv /nexus-git/nexus /out \
        && mv /nexus-git/storage/migrations /out/nexus-migrations \
        && rm -rf /nexus-git; \
    else \
        wget https://github.com/oasisprotocol/nexus/releases/download/v${NEXUS_VERSION}/oasis_nexus_${NEXUS_VERSION}_linux_amd64.tar.gz \
        && tar -zxvf oasis_nexus_${NEXUS_VERSION}_linux_amd64.tar.gz \
        && strip -s /oasis_nexus_${NEXUS_VERSION}_linux_amd64/nexus \
        && mv /oasis_nexus_${NEXUS_VERSION}_linux_amd64/nexus /out \
        && rm -rf /oasis_nexus_${NEXUS_VERSION}_linux_amd64* && \
        # We also need to fetch the storage migrations.
        git clone https://github.com/oasisprotocol/nexus.git nexus-git --branch v${NEXUS_VERSION} --depth 1 \
        && mv /nexus-git/storage/migrations /out/nexus-migrations \
        && rm -rf nexus-git; \
    fi \
    \
    && cd / \
    && git clone https://github.com/oasisprotocol/explorer.git explorer --depth 1 \
    && cd explorer \
    && git fetch origin ${EXPLORER_BRANCH} --depth 1 \
    && git checkout FETCH_HEAD \
    && mv /out/nexus.env.production .env.production \
    && echo "REACT_APP_LOCALNET_SAPPHIRE=true\nREACT_APP_LOCALNET_EMERALD=false" >> .env.production \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt install -y nodejs && apt clean \
    && npm install -g yarn \
    && yarn install \
    && NODE_ENV=production yarn build \
    && mv build /out/explorer \
    && rm -rf /explorer


# Build sapphire-localnet
FROM postgres:16-bookworm

# Docker-specific variables.
ARG PARATIME_VERSION
ARG PARATIME_NAME
ENV PARATIME_VERSION=${PARATIME_VERSION}
ENV PARATIME_NAME=${PARATIME_NAME}
ENV GATEWAY__CHAIN_ID=0x5afd

# start.sh and spinup-oasis-stack.sh ENV variables.
# Make sure these match what is produced in the build step above!
ENV OASIS_NODE_LOG_LEVEL=warn
ENV OASIS_NODE_BINARY=/oasis-node
ENV OASIS_NET_RUNNER_BINARY=/oasis-net-runner
ENV OASIS_NODE_DATADIR=/serverdir/node
ENV OASIS_WEB3_GATEWAY_BINARY=/oasis-web3-gateway
ENV ENVOY_BINARY=/usr/bin/envoy
ENV ENVOY_CONFIG_FILE=/envoy.yml
ENV OASIS_WEB3_GATEWAY_CONFIG_FILE=/localnet.yml
ENV PARATIME_BINARY=/ronl.elf
ENV KEYMANAGER_BINARY=/simple-keymanager
ENV OASIS_CLI_BINARY=/oasis
ENV OASIS_NEXUS_BINARY=/nexus
ENV OASIS_NEXUS_CONFIG_FILE=/nexus.yml
ENV OASIS_NEXUS_MIGRATIONS=/nexus-migrations
ENV OASIS_EXPLORER_DIR=/explorer
ENV OASIS_EXPLORER_NGINX_CONFIG_FILE=/explorer.nginx.conf
ENV OASIS_UNSAFE_SKIP_AVR_VERIFY=1
ENV OASIS_UNSAFE_ALLOW_DEBUG_ENCLAVES=1
ENV OASIS_UNSAFE_MOCK_SGX=1

ARG VERSION

RUN apt update && apt install -y --no-install-recommends bash libseccomp2 jq curl ca-certificates unzip binutils file \
    && su -c "POSTGRES_USER=postgres POSTGRES_PASSWORD=postgres /usr/local/bin/docker-entrypoint.sh postgres &" postgres \
    # Explorer frontend deps.
    && apt install -y --no-install-recommends nginx \
    # Install Envoy.
    && curl -fsSL https://apt.envoyproxy.io/signing.key | gpg --dearmor -o /etc/apt/keyrings/envoy-keyring.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/envoy-keyring.gpg] https://apt.envoyproxy.io bookworm main" | tee /etc/apt/sources.list.d/envoy.list \
    && apt update && apt install -y --no-install-recommends envoy \
    # Strip all binaries to save space.
    && find / -xdev -type f -executable -print0 | xargs -0 file --mime-type | egrep 'application/(x-executable|x-sharedlib)' | cut -d':' -f1 | xargs strip -s \
    # Clean up.
    && apt purge -y binutils file \
    && apt autoremove -y && apt clean \
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* \
    && rm -rf /etc/apt/sources.list.d/envoy.list \
    && rm -f /etc/apt/keyrings/envoy-keyring.gpg \
    && rm -rf /usr/share/doc

# Copy all results from build step.
COPY --from=oasis-core-dev /out /

# Create symlinks for convenience and a dir to put all your rofls into,
# also print version info.
RUN ln -s ${OASIS_NODE_BINARY} ${OASIS_CLI_BINARY} /usr/local/bin \
    && mkdir /rofls \
    && echo "${VERSION}" > /VERSION \
    && echo \
    && ls -la / \
    && echo \
    && echo "*** Oasis Node:" \
    && ${OASIS_NODE_BINARY} --version \
    && echo \
    && echo "*** Oasis CLI:" \
    && ${OASIS_CLI_BINARY} --version \
    && echo

# Envoy proxy port.
EXPOSE 8544/tcp
# Web3 gateway http and ws ports.
EXPOSE 8545/tcp
EXPOSE 8546/tcp
# Nexus port.
EXPOSE 8547/tcp
# Explorer port.
EXPOSE 8548/tcp

USER root
ENTRYPOINT ["/start.sh"]
