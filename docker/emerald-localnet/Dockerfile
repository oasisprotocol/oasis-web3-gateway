# Build oasis-web3-gateway
FROM golang:1.22.3 AS oasis-web3-gateway

COPY . /go/oasis-web3-gateway
RUN cd oasis-web3-gateway && make && strip -S -x oasis-web3-gateway docker/common/oasis-deposit/oasis-deposit

# Build emerald-localnet
FROM postgres:16-alpine
RUN apk add --no-cache bash gcompat libseccomp jq binutils \
	&& su -c "POSTGRES_USER=postgres POSTGRES_PASSWORD=postgres /usr/local/bin/docker-entrypoint.sh postgres &" postgres

# Docker-specific variables
ENV OASIS_CORE_VERSION=24.2
ENV OASIS_CLI_VERSION=0.10.0
ENV PARATIME_VERSION=11.0.0-testnet
ENV PARATIME_NAME=emerald
ENV GATEWAY__CHAIN_ID=0xa514

# start.sh and spinup-oasis-stack.sh ENV variables.
ENV OASIS_NODE=/oasis-node
ENV OASIS_NET_RUNNER=/oasis-net-runner
ENV OASIS_NODE_DATADIR=/serverdir/node
ENV OASIS_WEB3_GATEWAY=/oasis-web3-gateway
ENV OASIS_DEPOSIT=/oasis-deposit
ENV OASIS_WEB3_GATEWAY_CONFIG_FILE=/localnet.yml
ENV PARATIME=/runtime.elf
ENV KEYMANAGER_BINARY=""

ARG VERSION

# oasis-web3-gateway binary, config, spinup-* scripts and staking_genesis.json.
COPY --from=oasis-web3-gateway /go/oasis-web3-gateway/oasis-web3-gateway ${OASIS_WEB3_GATEWAY}
COPY --from=oasis-web3-gateway /go/oasis-web3-gateway/docker/common/oasis-deposit/oasis-deposit ${OASIS_DEPOSIT}
COPY docker/common/localnet.yml ${OASIS_WEB3_GATEWAY_CONFIG_FILE}
COPY docker/common/start.sh /
COPY tests/tools/* /

# Configure oasis-node and oasis-net-runner.
RUN wget --quiet "https://github.com/oasisprotocol/oasis-core/releases/download/v${OASIS_CORE_VERSION}/oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz"  \
	&& tar xfvz "oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz" \
	&& mkdir -p "$(dirname ${OASIS_NODE})" "$(dirname ${OASIS_NET_RUNNER})" \
	&& mv "oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-node" "${OASIS_NODE}" \
	&& mv "oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-net-runner" "${OASIS_NET_RUNNER}" \
	&& rm "oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz" \
	&& rm -rf "oasis_core_${OASIS_CORE_VERSION}_linux_amd64" \
	&& echo "" \
	&& echo "Configure the ParaTime." \
	&& mkdir -p "$(dirname ${PARATIME})" \
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
	&& strip -S -x /oasis-net-runner /oasis-node /oasis

# Web3 gateway http and ws ports.
EXPOSE 8545/tcp
EXPOSE 8546/tcp

USER root
ENTRYPOINT ["/start.sh"]
