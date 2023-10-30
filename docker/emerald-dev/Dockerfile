# Build oasis-web3-gateway
FROM golang:1.21 AS oasis-web3-gateway

COPY . /go/oasis-web3-gateway
RUN cd oasis-web3-gateway && make build

# Build emerald-dev
FROM ubuntu:22.04

# Docker-specific variables
ENV OASIS_CORE_VERSION=23.0.1
ENV OASIS_CLI_VERSION=0.6.0
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
ARG DEBIAN_FRONTEND=noninteractive

# Install Postgresql and other tools packaged by Ubuntu.
RUN apt update -qq \
	&& apt dist-upgrade -qq -y \
	&& apt install jq postgresql unzip ca-certificates wget -y \
	&& rm -rf /var/lib/apt/lists/* \
	&& echo "" \
	&& echo "Initialize PostgreSQL database, permissions need to be setup" \
	&& chown -R postgres:postgres /etc/postgresql /var/run/postgresql /var/log/postgresql /var/lib/postgresql/ \
	&& chown postgres:ssl-cert /etc/ssl/private/ \
	&& chown postgres:postgres /etc/ssl/private/ssl-cert-snakeoil.key \
	&& chmod 600               /etc/ssl/private/ssl-cert-snakeoil.key  \
	&& /etc/init.d/postgresql start \
	&& su -c "psql --command \"ALTER USER postgres WITH SUPERUSER PASSWORD 'postgres';\"" postgres

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
	&& echo "${VERSION}" > /VERSION

# Web3 gateway http and ws ports.
EXPOSE 8545/tcp
EXPOSE 8546/tcp

USER root
ENTRYPOINT ["/start.sh"]
