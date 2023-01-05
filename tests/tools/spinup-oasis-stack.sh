#!/bin/bash

set -euo pipefail

# This script spins up local oasis node configured with the provided EVM ParaTime.
# Supported ENV Variables:
# - OASIS_NODE: path to oasis-node binary
# - OASIS_NET_RUNNER: path to oasis-net-runner binary
# - PARATIME: path to ParaTime binary (inside .orc bundle)
# - PARATIME_VERSION: version of the binary. e.g. 3.0.0
# - OASIS_NODE_DATADIR: path to temporary oasis-node data dir e.g. /tmp/oasis-localnet
# - KEYMANAGER_BINARY: path to key manager binary e.g. simple-keymanager

function paratime_ver {
  echo $PARATIME_VERSION | cut -d \- -f 1 | cut -d + -f 1 | cut -d . -f $1
}
export FIXTURE_FILE="${OASIS_NODE_DATADIR}/fixture.json"
export STAKING_GENESIS_FILE="$(dirname "$0")/staking_genesis.json"

rm -rf "$OASIS_NODE_DATADIR"
mkdir -p "$OASIS_NODE_DATADIR"

# Prepare configuration for oasis-node (fixture).
${OASIS_NET_RUNNER} dump-fixture \
  --fixture.default.node.binary "${OASIS_NODE}" \
  --fixture.default.deterministic_entities \
  --fixture.default.fund_entities \
  --fixture.default.num_entities 2 \
  --fixture.default.keymanager.binary "${KEYMANAGER_BINARY:-}" \
  --fixture.default.runtime.binary "${PARATIME}" \
  --fixture.default.runtime.provisioner "unconfined" \
  --fixture.default.runtime.version "$(paratime_ver 1).$(paratime_ver 2).$(paratime_ver 3)" \
  --fixture.default.halt_epoch 100000 \
  --fixture.default.staking_genesis "${STAKING_GENESIS_FILE}" >"$FIXTURE_FILE"

# Determine compute runtime ID.
RT_IDX=0
if [ ! -z "${KEYMANAGER_BINARY:-}" ]; then
  RT_IDX=1
fi

# Enable expensive queries for testing.
jq "
  .clients[0].runtime_config.\"${RT_IDX}\".estimate_gas_by_simulating_contracts = true |
  .clients[0].runtime_config.\"${RT_IDX}\".allowed_queries = [{all_expensive: true}]
" "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"

# Whitelist compute node for key manager.
if [ ! -z "${KEYMANAGER_BINARY:-}" ]; then
  jq '.keymanagers[0].private_peer_pub_keys = ["pr+KLREDcBxpWgQ/80yUrHXbyhDuBDcnxzo3td4JiIo="]' "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
  mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"
fi

# Bump the batch size (default=1).
jq ".runtimes[${RT_IDX}].txn_scheduler.max_batch_size=20" "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"

jq ".runtimes[${RT_IDX}].txn_scheduler.max_batch_size_bytes=1048576" "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"

jq ".runtimes[${RT_IDX}].txn_scheduler.propose_batch_timeout=2" "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"

# Use a batch timeout of 1 second.
jq ".runtimes[${RT_IDX}].txn_scheduler.batch_flush_timeout=1000000000" "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"

# Run oasis-node.
${OASIS_NET_RUNNER} --fixture.file "$FIXTURE_FILE" --basedir "${OASIS_NODE_DATADIR}" --basedir.no_temp_dir
