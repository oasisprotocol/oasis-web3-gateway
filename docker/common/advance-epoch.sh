#!/usr/bin/env bash
#
# This script advances the mock epoch by 1 or by the number of epochs
# given as the first argument to this script.

set -eo pipefail

num_epochs="${1:-1}"

OASIS_NODE_SOCKET=${OASIS_NODE_DATADIR}/net-runner/network/client-0/internal.sock

if [[ "${BEACON_BACKEND:-mock}" != "mock" ]]; then
  echo "This command only works with the mock beacon backend."
  echo "Run the container with BEACON_BACKEND set to 'mock' to enable it."
  exit 1
fi

# Wait for the Oasis Node to start.
while [[ ! -S ${OASIS_NODE_SOCKET} ]]; do sleep 1; done

old_epoch=`${OASIS_NODE_BINARY} control status -a unix:${OASIS_NODE_SOCKET} | jq '.consensus.latest_epoch'`
new_epoch=$((old_epoch + num_epochs))
${OASIS_NODE_BINARY} debug control set-epoch --epoch ${new_epoch} -a unix:${OASIS_NODE_SOCKET}

echo "Epoch advanced from ${old_epoch} to ${new_epoch}."
exit 0
