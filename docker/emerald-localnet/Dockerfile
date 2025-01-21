# Build oasis-web3-gateway
FROM golang:1.23.3 AS oasis-web3-gateway

COPY . /go/oasis-web3-gateway
RUN cd oasis-web3-gateway && make && strip -S -x oasis-web3-gateway

FROM ghcr.io/oasisprotocol/oasis-core-dev:stable-24.2.x AS oasis-core-dev

ARG NEXUS_VERSION=0.5.3
RUN wget https://github.com/oasisprotocol/nexus/releases/download/v${NEXUS_VERSION}/oasis_nexus_${NEXUS_VERSION}_linux_amd64.tar.gz \
    && tar -zxvf oasis_nexus_${NEXUS_VERSION}_linux_amd64.tar.gz \
    && strip -S -x /oasis_nexus_${NEXUS_VERSION}_linux_amd64/nexus \
    && mv /oasis_nexus_${NEXUS_VERSION}_linux_amd64/nexus / \
    && rm -rf /oasis_nexus_${NEXUS_VERSION}_linux_amd64* && \
    # We also need to fetch the storage migrations.
    git clone https://github.com/oasisprotocol/nexus.git nexus-git --branch v${NEXUS_VERSION} --depth 1 \
    && mv /nexus-git/storage/migrations /nexus-migrations \
    && rm -rf nexus-git
# ARG NEXUS_VERSION=main
# RUN git clone https://github.com/oasisprotocol/nexus.git nexus-git --depth 1 \
#     && cd nexus-git \
#     && git fetch --depth 1 origin ${NEXUS_VERSION} \
#     && git checkout ${NEXUS_VERSION} \
#     && go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@v1.12 \
#     && make codegen-go \
#     && go build \
#     && strip -S -x /nexus-git/nexus \
#     && mv /nexus-git/nexus / \
#     && mv /nexus-git/storage/migrations /nexus-migrations \
#     && rm -rf /nexus-git

ARG EXPLORER_REPO=https://github.com/oasisprotocol/explorer.git
ARG EXPLORER_VERSION=v1.15.1
COPY docker/common/nexus.env.production /nexus.env.production
RUN git clone ${EXPLORER_REPO} explorer --depth 1 \
    && cd explorer \
    && git fetch origin ${EXPLORER_VERSION} --depth 1 \
    && git checkout FETCH_HEAD \
    && mv /nexus.env.production .env.production \
    && echo "REACT_APP_LOCALNET_EMERALD=true\nREACT_APP_LOCALNET_SAPPHIRE=false" >> .env.production \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt install -y nodejs \
    && npm install -g yarn \
    && yarn install \
    && NODE_ENV=production yarn build


# Build emerald-localnet
FROM postgres:16-bookworm
RUN apt update && apt install -y bash libseccomp2 unzip jq binutils curl wget nginx && apt clean \
    && su -c "POSTGRES_USER=postgres POSTGRES_PASSWORD=postgres /usr/local/bin/docker-entrypoint.sh postgres &" postgres \
	# Explorer frontend deps.
    && apt install -y --no-install-recommends nginx \
    # Install Envoy.
    && curl -fsSL https://apt.envoyproxy.io/signing.key | gpg --dearmor -o /etc/apt/keyrings/envoy-keyring.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/envoy-keyring.gpg] https://apt.envoyproxy.io bookworm main" | tee /etc/apt/sources.list.d/envoy.list \
    && apt update \
    && apt install -y envoy \
    # Clean up.
    && apt clean \
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* \
    && rm -rf /etc/apt/sources.list.d/envoy.list \
    && rm -f /etc/apt/keyrings/envoy-keyring.gpg

# Docker-specific variables
ENV OASIS_CORE_VERSION=24.3.1
ENV OASIS_CLI_VERSION=0.10.3
ENV PARATIME_VERSION=11.0.0-testnet
ENV PARATIME_NAME=emerald
ENV GATEWAY__CHAIN_ID=0xa514

# start.sh and spinup-oasis-stack.sh ENV variables.
ENV OASIS_NODE_BINARY=/oasis-node
ENV OASIS_NET_RUNNER_BINARY=/oasis-net-runner
ENV OASIS_NODE_DATADIR=/serverdir/node
ENV OASIS_WEB3_GATEWAY_BINARY=/oasis-web3-gateway
ENV OASIS_WEB3_GATEWAY_CONFIG_FILE=/localnet.yml
ENV ENVOY_BINARY=/usr/bin/envoy
ENV ENVOY_CONFIG_FILE=/envoy.yml
ENV PARATIME_BINARY=/runtime.elf
ENV KEYMANAGER_BINARY=""
ENV OASIS_CLI_BINARY=/oasis
ENV OASIS_NEXUS_BINARY=/nexus
ENV OASIS_NEXUS_CONFIG_FILE=/nexus.yml
ENV OASIS_NEXUS_MIGRATIONS=/nexus-migrations
ENV OASIS_EXPLORER_DIR=/explorer
ENV OASIS_EXPLORER_NGINX_CONFIG_FILE=/explorer.nginx.conf

ARG VERSION

# oasis-web3-gateway binary, config, spinup-* scripts and staking_genesis.json.
COPY --from=oasis-web3-gateway /go/oasis-web3-gateway/oasis-web3-gateway ${OASIS_WEB3_GATEWAY_BINARY}
COPY docker/common/localnet.yml ${OASIS_WEB3_GATEWAY_CONFIG_FILE}
COPY docker/common/nexus.yml ${OASIS_NEXUS_CONFIG_FILE}
COPY docker/common/envoy.yml ${ENVOY_CONFIG_FILE}
COPY docker/common/explorer.nginx.conf ${OASIS_EXPLORER_NGINX_CONFIG_FILE}
COPY docker/common/start.sh /
COPY docker/common/wait-container-ready.sh /
COPY tests/tools/* /
COPY --from=oasis-core-dev /nexus ${OASIS_NEXUS_BINARY}
COPY --from=oasis-core-dev /nexus-migrations ${OASIS_NEXUS_MIGRATIONS}
COPY --from=oasis-core-dev /explorer/build ${OASIS_EXPLORER_DIR}

# Create symbolic links for convenience.
RUN ln -s ${OASIS_NODE_BINARY} ${OASIS_CLI_BINARY} /usr/local/bin

# Configure oasis-node and oasis-net-runner.
RUN wget --quiet "https://github.com/oasisprotocol/oasis-core/releases/download/v${OASIS_CORE_VERSION}/oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz"  \
	&& tar xfvz "oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz" \
	&& mkdir -p "$(dirname ${OASIS_NODE_BINARY})" "$(dirname ${OASIS_NET_RUNNER_BINARY})" \
	&& mv "oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-node" "${OASIS_NODE_BINARY}" \
	&& mv "oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-net-runner" "${OASIS_NET_RUNNER_BINARY}" \
	&& rm "oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz" \
	&& rm -rf "oasis_core_${OASIS_CORE_VERSION}_linux_amd64" \
	&& echo "" \
	&& echo "Configure the ParaTime." \
	&& mkdir -p "$(dirname ${PARATIME_BINARY})" \
	&& wget --quiet "https://github.com/oasisprotocol/${PARATIME_NAME}-paratime/releases/download/v${PARATIME_VERSION}/localnet-${PARATIME_NAME}-paratime.orc" -O "/paratime.orc" \
	&& unzip "paratime.orc" \
	&& chmod a+x "runtime.elf" \
	&& rm "paratime.orc" \
	&& echo "" \
	&& echo "Configure oasis-cli." \
	&& wget --quiet "https://github.com/oasisprotocol/cli/releases/download/v${OASIS_CLI_VERSION}/oasis_cli_${OASIS_CLI_VERSION}_linux_amd64.tar.gz" \
	&& tar -xvf "/oasis_cli_${OASIS_CLI_VERSION}_linux_amd64.tar.gz" \
	&& mv "/oasis_cli_${OASIS_CLI_VERSION}_linux_amd64/oasis" / \
	&& rm -rf "/oasis_cli_${OASIS_CLI_VERSION}_linux_amd64/" \
	&& rm -rf "/oasis_cli_${OASIS_CLI_VERSION}_linux_amd64.tar.gz" \
	&& echo "" \
	&& echo "Write VERSION information." \
	&& echo "${VERSION}" > /VERSION \
	&& strip -S -x ${OASIS_NET_RUNNER_BINARY} ${OASIS_NODE_BINARY} ${OASIS_CLI_BINARY}


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
