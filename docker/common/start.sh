#!/bin/bash

# This script spins up local oasis stack by running the spinup-oasis-stack.sh,
# starts the web3 gateway on top of it, and deposits to a test account on the
# ParaTime.
# Mandatory ENV Variables:
# - all ENV variables required by spinup-oasis-stack.sh
# - OASIS_WEB3_GATEWAY: path to oasis-web3-gateway binary
# - OASIS_WEB3_GATEWAY_CONFIG_FILE: path to oasis-web3-gateway config file
# - OASIS_DEPOSIT: path to oasis-deposit binary
# - SAPPHIRE_BACKEND: uses 'mock' backend by default
# - OASIS_SINGLE_COMPUTE_NODE: (default: true) if non-empty only run a single compute node

export OASIS_SINGLE_COMPUTE_NODE=${OASIS_SINGLE_COMPUTE_NODE-1}
OASIS_WEB3_GATEWAY_VERSION=$(${OASIS_WEB3_GATEWAY} -v | head -n1 | cut -d " " -f 3 | sed -r 's/^v//')
OASIS_CORE_VERSION=$(${OASIS_NODE} -v | head -n1 | cut -d " " -f 3 | sed -r 's/^v//')
VERSION=$(cat /VERSION)
echo "${PARATIME_NAME}-dev ${VERSION} (oasis-core: ${OASIS_CORE_VERSION}, ${PARATIME_NAME}-paratime: ${PARATIME_VERSION}, oasis-web3-gateway: ${OASIS_WEB3_GATEWAY_VERSION})"
echo

if [[ ${PARATIME_NAME} == 'sapphire' ]]; then
export SAPPHIRE_BACKEND=${SAPPHIRE_BACKEND:-mock}
else
export SAPPHIRE_BACKEND=default
fi

OASIS_NODE_SOCKET=${OASIS_NODE_DATADIR}/net-runner/network/client-0/internal.sock

set -euo pipefail

function cleanup {
	kill -9 $OASIS_WEB3_GATEWAY_PID
	kill -9 $OASIS_NODE_PID
}

trap cleanup INT TERM EXIT

echo " * Starting oasis-net-runner with ${PARATIME_NAME}"
/spinup-oasis-stack.sh 2>1 &>/var/log/spinup-oasis-stack.log &
OASIS_NODE_PID=$!

chown -R postgres:postgres /etc/postgresql /var/run/postgresql /var/log/postgresql /var/lib/postgresql/
chown postgres:ssl-cert /etc/ssl/private/
chown postgres:postgres /etc/ssl/private/ssl-cert-snakeoil.key
chmod 600               /etc/ssl/private/ssl-cert-snakeoil.key
/etc/init.d/postgresql start

echo " * Starting oasis-web3-gateway"
# Wait for oasis-node socket before starting web3 gateway.
while ! [[ -S ${OASIS_NODE_SOCKET} ]]; do sleep 1; done

${OASIS_WEB3_GATEWAY} --config ${OASIS_WEB3_GATEWAY_CONFIG_FILE} 2>1 &>/var/log/oasis-web3-gateway.log &
OASIS_WEB3_GATEWAY_PID=$!

# Wait for compute nodes before initiating deposit.
echo -n " * Bootstrapping network (this might take a minute)"
if [[ ${SAPPHIRE_BACKEND} == 'mock' ]]; then
	echo -n .
	${OASIS_NODE} debug control wait-nodes -n 2 -a unix:${OASIS_NODE_SOCKET}

	echo -n .
	${OASIS_NODE} debug control set-epoch --epoch 1 -a unix:${OASIS_NODE_SOCKET}

	echo -n .
	${OASIS_NODE} debug control set-epoch --epoch 2 -a unix:${OASIS_NODE_SOCKET}
else
	${OASIS_NODE} debug control wait-ready -a unix:${OASIS_NODE_SOCKET}
fi

echo
echo " * Populating accounts"
echo
${OASIS_DEPOSIT} -sock unix:${OASIS_NODE_SOCKET} "$@"

echo
echo "WARNING: The chain is running in ephemeral mode. State will be lost after restart!"
echo
echo "Listening on http://localhost:8545 and ws://localhost:8546"

if [[ ${SAPPHIRE_BACKEND} == 'mock' ]]; then
	# Run background task to switch epochs every 5 minutes
	while true; do
		sleep $((60*10))
		epoch=`${OASIS_NODE} control status -a unix:${OASIS_NODE_SOCKET} | jq '.consensus.latest_epoch'`
		epoch=$((epoch + 1))
		${OASIS_NODE} debug control set-epoch --epoch $epoch -a unix:${OASIS_NODE_SOCKET}
	done
else
	wait
fi
