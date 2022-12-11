#!/bin/bash

# This script spins up local oasis stack by running the spinup-oasis-stack.sh,
# starts the emerald web3 gateway on top of it, and deposits to a test account on Emerald.
# Mandatory ENV Variables:
# - all ENV variables required by spinup-oasis-stack.sh
# - EMERALD_WEB3_GATEWAY: path to emerald-web3-gateway binary
# - EMERALD_WEB3_GATEWAY_CONFIG_FILE: path to emerald-web3-gateway config file
# - OASIS_DEPOSIT: path to oasis-deposit binary

EMERALD_WEB3_GATEWAY_VERSION=$(${EMERALD_WEB3_GATEWAY} -v | head -n1 | cut -d " " -f 3 | sed -r 's/^v//')
OASIS_CORE_VERSION=$(${OASIS_NODE} -v | head -n1 | cut -d " " -f 3 | sed -r 's/^v//')
VERSION=$(cat /VERSION)
echo "emerald-dev ${VERSION} (oasis-core: ${OASIS_CORE_VERSION}, emerald-paratime: ${EMERALD_PARATIME_VERSION}, emerald-web3-gateway: ${EMERALD_WEB3_GATEWAY_VERSION})"
echo

OASIS_NODE_SOCKET=${OASIS_NODE_DATADIR}/net-runner/network/client-0/internal.sock

set -euo pipefail

function cleanup {
	kill -9 $EMERALD_WEB3_GATEWAY_PID
	kill -9 $OASIS_NODE_PID
}

trap cleanup INT TERM EXIT

echo "Starting oasis-net-runner with Emerald ParaTime..."
/spinup-oasis-stack.sh 2>1 &>/var/log/spinup-oasis-stack.log &
OASIS_NODE_PID=$!

echo "Starting postgresql..."
/etc/init.d/postgresql start >/dev/null

echo "Starting emerald-web3-gateway..."
# Wait for oasis-node socket before starting web3 gateway.
while ! [[ -S ${OASIS_NODE_SOCKET} ]]; do sleep 1; done

${EMERALD_WEB3_GATEWAY} --config ${EMERALD_WEB3_GATEWAY_CONFIG_FILE} 2>1 &>/var/log/emerald-web3-gateway.log &
EMERALD_WEB3_GATEWAY_PID=$!

echo "Populating account(s) (this might take a moment)..."
echo

# Wait for compute nodes before initiating deposit.
${OASIS_NODE} debug control wait-ready -a unix:${OASIS_NODE_SOCKET}

${OASIS_DEPOSIT} -sock unix:${OASIS_NODE_SOCKET} "$@"

echo
echo "WARNING: Emerald is running in ephemeral mode. The chain state will be lost after you quit!"
echo
echo "Listening on http://localhost:8545 and ws://localhost:8546"

wait
