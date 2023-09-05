# Build oasis-web3-gateway
FROM golang:1.20 AS oasis-web3-gateway

COPY . /go/oasis-web3-gateway
RUN cd oasis-web3-gateway && make build

# Build simple-keymanager
FROM oasisprotocol/oasis-core-dev:stable-22.2.x AS oasis-core-dev

ENV OASIS_UNSAFE_SKIP_KM_POLICY=1

RUN git clone https://github.com/oasisprotocol/oasis-core.git --branch stable/22.2.x --depth 1 \
    && cd oasis-core/tests/runtimes/simple-keymanager \
    && cargo build

# Build sapphire-dev
FROM ubuntu:22.04

# Docker-specific variables
ENV OASIS_CORE_VERSION=22.2.11
ENV PARATIME_VERSION=0.6.2-testnet
ENV PARATIME_NAME=sapphire
ENV GATEWAY__CHAIN_ID=0x5afd

# start.sh and spinup-oasis-stack.sh ENV variables.
ENV OASIS_NODE=/oasis-node
ENV OASIS_NET_RUNNER=/oasis-net-runner
ENV OASIS_NODE_DATADIR=/serverdir/node
ENV OASIS_WEB3_GATEWAY=/oasis-web3-gateway
ENV OASIS_DEPOSIT=/oasis-deposit
ENV OASIS_WEB3_GATEWAY_CONFIG_FILE=/localnet.yml
ENV PARATIME=/runtime.elf
ENV KEYMANAGER_BINARY=/simple-keymanager
ENV OASIS_UNSAFE_SKIP_AVR_VERIFY=1
ENV OASIS_UNSAFE_SKIP_KM_POLICY=1
ENV OASIS_UNSAFE_ALLOW_DEBUG_ENCLAVES=1

ARG VERSION
ARG DEBIAN_FRONTEND=noninteractive

# Install Postgresql and other tools packaged by Ubuntu.
RUN apt update -qq \
	&& apt dist-upgrade -qq -y \
	&& apt install jq postgresql unzip -y \
	&& rm -rf /var/lib/apt/lists/*

# simple-keymanager
COPY --from=oasis-core-dev /oasis-core/target/debug/simple-keymanager ${KEYMANAGER_BINARY}

# oasis-web3-gateway binary, config, spinup-* scripts and staking_genesis.json.
COPY --from=oasis-web3-gateway /go/oasis-web3-gateway/oasis-web3-gateway ${OASIS_WEB3_GATEWAY}
COPY --from=oasis-web3-gateway /go/oasis-web3-gateway/docker/common/oasis-deposit/oasis-deposit ${OASIS_DEPOSIT}
COPY docker/common/localnet.yml ${OASIS_WEB3_GATEWAY_CONFIG_FILE}
COPY docker/common/start.sh /
COPY tests/tools/* /

# Configure oasis-node and oasis-net-runner.
ADD "https://github.com/oasisprotocol/oasis-core/releases/download/v${OASIS_CORE_VERSION}/oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz" /
RUN tar xfvz "oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz" \
	&& mkdir -p "$(dirname ${OASIS_NODE})" "$(dirname ${OASIS_NET_RUNNER})" \
	&& mv "oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-node" "${OASIS_NODE}" \
	&& mv "oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-net-runner" "${OASIS_NET_RUNNER}"

# Configure the ParaTime.
RUN mkdir -p "$(dirname ${PARATIME})"
ADD "https://github.com/oasisprotocol/${PARATIME_NAME}-paratime/releases/download/v${PARATIME_VERSION}/localnet-${PARATIME_NAME}-paratime.orc" "/paratime.orc"
RUN unzip "paratime.orc" \
    && chmod a+x "runtime.elf"

# Write VERSION information.
RUN echo "${VERSION}" > /VERSION

# Initialize PostgreSQL database, permissions need to be setup
USER root
RUN chown -R postgres:postgres /etc/postgresql /var/run/postgresql /var/log/postgresql /var/lib/postgresql/ \
	&& chown postgres:ssl-cert /etc/ssl/private/ \
	&& chown postgres:postgres /etc/ssl/private/ssl-cert-snakeoil.key \
	&& chmod 600               /etc/ssl/private/ssl-cert-snakeoil.key  \
	&& /etc/init.d/postgresql start \
	&& su -c "psql --command \"ALTER USER postgres WITH SUPERUSER PASSWORD 'postgres';\"" postgres

# Web3 gateway http and ws ports.
EXPOSE 8545/tcp
EXPOSE 8546/tcp

USER root
ENTRYPOINT ["/start.sh"]
