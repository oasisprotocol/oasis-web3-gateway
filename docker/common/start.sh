#!/bin/bash

# This script spins up local oasis stack by running the spinup-oasis-stack.sh,
# starts the web3 gateway on top of it, and deposits to a test account on the
# ParaTime.
# Mandatory ENV Variables:
# - all ENV variables required by spinup-oasis-stack.sh
# - OASIS_WEB3_GATEWAY: path to oasis-web3-gateway binary
# - OASIS_WEB3_GATEWAY_CONFIG_FILE: path to oasis-web3-gateway config file
# - OASIS_DEPOSIT: path to oasis-deposit binary

OASIS_WEB3_GATEWAY_VERSION=$(${OASIS_WEB3_GATEWAY} -v | head -n1 | cut -d " " -f 3 | sed -r 's/^v//')
OASIS_CORE_VERSION=$(${OASIS_NODE} -v | head -n1 | cut -d " " -f 3 | sed -r 's/^v//')
VERSION=$(cat /VERSION)
echo "${PARATIME_NAME}-dev ${VERSION} (oasis-core: ${OASIS_CORE_VERSION}, ${PARATIME_NAME}-paratime: ${PARATIME_VERSION}, oasis-web3-gateway: ${OASIS_WEB3_GATEWAY_VERSION})"
echo

OASIS_NODE_SOCKET=${OASIS_NODE_DATADIR}/net-runner/network/client-0/internal.sock

set -euo pipefail

function cleanup {
	kill -9 $OASIS_WEB3_GATEWAY_PID
	kill -9 $OASIS_NODE_PID
}

trap cleanup INT TERM EXIT

echo "Starting oasis-net-runner with ${PARATIME_NAME}..."
/spinup-oasis-stack.sh 2>1 &>/var/log/spinup-oasis-stack.log &
OASIS_NODE_PID=$!

echo "Starting postgresql..."
/etc/init.d/postgresql start >/dev/null

echo "Starting oasis-web3-gateway..."
# Wait for oasis-node socket before starting web3 gateway.
while ! [[ -S ${OASIS_NODE_SOCKET} ]]; do sleep 1; done

${OASIS_WEB3_GATEWAY} --config ${OASIS_WEB3_GATEWAY_CONFIG_FILE} 2>1 &>/var/log/oasis-web3-gateway.log &
OASIS_WEB3_GATEWAY_PID=$!

echo "Bootstrapping network and populating account(s) (this might take a minute)..."
echo

# Wait for compute nodes before initiating deposit.
${OASIS_NODE} debug control wait-ready -a unix:${OASIS_NODE_SOCKET}

${OASIS_DEPOSIT} -sock unix:${OASIS_NODE_SOCKET} "$@"

echo
echo "WARNING: The chain is running in ephemeral mode. State will be lost after restart!"
echo
echo "Listening on http://localhost:8545 and ws://localhost:8546"

wait
