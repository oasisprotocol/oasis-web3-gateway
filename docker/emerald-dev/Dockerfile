FROM golang:1.17 AS emerald-web3-gateway

COPY . /go/emerald-web3-gateway
RUN cd emerald-web3-gateway && make build

FROM ubuntu:20.04

ENV OASIS_CORE_VERSION=22.1.8
ENV EMERALD_PARATIME_VERSION=9.0.1-testnet

ENV OASIS_NODE=/oasis-node
ENV OASIS_NET_RUNNER=/oasis-net-runner
ENV OASIS_NODE_DATADIR=/serverdir/node
ENV EMERALD_PARATIME=/emerald-paratime
ENV EMERALD_WEB3_GATEWAY=/emerald-web3-gateway
ENV EMERALD_WEB3_GATEWAY_CONFIG_FILE=/emerald-dev.yml
ENV OASIS_DEPOSIT=/oasis-deposit

ARG VERSION
ARG DEBIAN_FRONTEND=noninteractive

# Install Postgresql and other tools packaged by Ubuntu.
RUN apt update -qq \
	&& apt dist-upgrade -qq -y \
	&& apt install jq postgresql unzip -y \
	&& rm -rf /var/lib/apt/lists/*

# emerald-web3-gateway binary, config, spinup-* scripts and staking_genesis.json.
COPY --from=emerald-web3-gateway /go/emerald-web3-gateway/emerald-web3-gateway ${EMERALD_WEB3_GATEWAY}
COPY --from=emerald-web3-gateway /go/emerald-web3-gateway/docker/emerald-dev/oasis-deposit/oasis-deposit ${OASIS_DEPOSIT}
COPY docker/emerald-dev/emerald-dev.yml ${EMERALD_WEB3_GATEWAY_CONFIG_FILE}
COPY docker/emerald-dev/start-emerald-dev.sh /
COPY tests/tools/* /

# Configure oasis-node and oasis-net-runner.
ADD "https://github.com/oasisprotocol/oasis-core/releases/download/v${OASIS_CORE_VERSION}/oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz" /
RUN tar xfvz "oasis_core_${OASIS_CORE_VERSION}_linux_amd64.tar.gz" \
	&& mkdir -p "$(dirname ${OASIS_NODE})" "$(dirname ${OASIS_NET_RUNNER})" \
	&& mv "oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-node" "${OASIS_NODE}" \
	&& mv "oasis_core_${OASIS_CORE_VERSION}_linux_amd64/oasis-net-runner" "${OASIS_NET_RUNNER}"

# Configure Emerald ParaTime.
RUN mkdir -p "$(dirname ${EMERALD_PARATIME})"
ADD "https://github.com/oasisprotocol/emerald-paratime/releases/download/v${EMERALD_PARATIME_VERSION}/emerald-paratime.orc" "${EMERALD_PARATIME}.orc"
RUN unzip "${EMERALD_PARATIME}.orc" \
	&& mv runtime.elf "${EMERALD_PARATIME}" \
	&& chmod a+x "${EMERALD_PARATIME}"

# Write VERSION information.
RUN echo "${VERSION}" > /VERSION

USER postgres
RUN /etc/init.d/postgresql start \
	&& psql --command "ALTER USER postgres WITH SUPERUSER PASSWORD 'postgres';"

# Web3 gateway http and ws ports.
EXPOSE 8545/tcp
EXPOSE 8546/tcp

USER root
ENTRYPOINT ["/start-emerald-dev.sh"]
