# Build oasis-web3-gateway
FROM golang:1.22.3 AS oasis-web3-gateway

COPY . /go/oasis-web3-gateway
RUN cd oasis-web3-gateway && make && strip -S -x oasis-web3-gateway docker/common/oasis-deposit/oasis-deposit

# Build emerald-localnet
FROM postgres:16-bookworm
RUN apt update && apt install -y bash libseccomp2 unzip jq binutils curl wget && apt clean \
    && su -c "POSTGRES_USER=postgres POSTGRES_PASSWORD=postgres /usr/local/bin/docker-entrypoint.sh postgres &" postgres \
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
ENV OASIS_CORE_VERSION=24.2
ENV OASIS_CLI_VERSION=0.10.1
ENV PARATIME_VERSION=11.0.0-testnet
ENV PARATIME_NAME=emerald
ENV GATEWAY__CHAIN_ID=0xa514

# start.sh and spinup-oasis-stack.sh ENV variables.
ENV OASIS_NODE_BINARY=/oasis-node
ENV OASIS_NET_RUNNER_BINARY=/oasis-net-runner
ENV OASIS_NODE_DATADIR=/serverdir/node
ENV OASIS_WEB3_GATEWAY_BINARY=/oasis-web3-gateway
ENV OASIS_DEPOSIT_BINARY=/oasis-deposit
ENV OASIS_WEB3_GATEWAY_CONFIG_FILE=/localnet.yml
ENV ENVOY_BINARY=/usr/bin/envoy
ENV ENVOY_CONFIG_FILE=/envoy.yml
ENV PARATIME_BINARY=/runtime.elf
ENV KEYMANAGER_BINARY=""
ENV OASIS_CLI_BINARY=/oasis

ARG VERSION

# oasis-web3-gateway binary, config, spinup-* scripts and staking_genesis.json.
COPY --from=oasis-web3-gateway /go/oasis-web3-gateway/oasis-web3-gateway ${OASIS_WEB3_GATEWAY_BINARY}
COPY --from=oasis-web3-gateway /go/oasis-web3-gateway/docker/common/oasis-deposit/oasis-deposit ${OASIS_DEPOSIT_BINARY}
COPY docker/common/localnet.yml ${OASIS_WEB3_GATEWAY_CONFIG_FILE}
COPY docker/common/envoy.yml ${ENVOY_CONFIG_FILE}
COPY docker/common/start.sh /
COPY docker/common/wait-container-ready.sh /
COPY tests/tools/* /

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

USER root
ENTRYPOINT ["/start.sh"]
