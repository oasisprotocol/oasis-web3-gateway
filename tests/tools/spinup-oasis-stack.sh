#!/bin/bash

set -euo pipefail

# This script spins up local oasis node configured with emerald paratime.
# Mandatory ENV Variables:
# - OASIS_NODE: path to oasis-node binary
# - OASIS_NET_RUNNER: path to oasis-net-runner binary
# - OASIS_EMERALD_PARATIME: path to emerald-paratime binary
# - OASIS_EMERALD_VERSION: emerald version of the binary. e.g. 3.0.0
# - OASIS_NODE_DATADIR: path to temprorary oasis-node data dir e.g. /tmp/eth-runtime-test

function emerald_ver {
  echo $OASIS_EMERALD_VERSION | cut -d \- -f 1 | cut -d + -f 1 | cut -d . -f $1
}
export FIXTURE_FILE="${OASIS_NODE_DATADIR}/fixture.json"
export STAKING_GENESIS_FILE="$(dirname "$0")/staking_genesis.json"

rm -rf "$OASIS_NODE_DATADIR"
mkdir "$OASIS_NODE_DATADIR"

# Prepare configuration for oasis-node (fixture).
${OASIS_NET_RUNNER} dump-fixture \
  --fixture.default.node.binary "${OASIS_NODE}" \
  --fixture.default.deterministic_entities \
  --fixture.default.fund_entities \
  --fixture.default.num_entities 2 \
  --fixture.default.keymanager.binary '' \
  --fixture.default.runtime.binary "${OASIS_EMERALD_PARATIME}" \
  --fixture.default.halt_epoch 100000 \
  --fixture.default.staking_genesis "${STAKING_GENESIS_FILE}" >"$FIXTURE_FILE"

# Enable expensive queries for testing.
jq '.clients[0].runtime_config."1".allow_expensive_queries = true' "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"

# Assign non-zero version to runtime, otherwise transactions will not be confirmed.
jq ".runtimes[1].version = {major:"$(emerald_ver 1)", minor:"$(emerald_ver 2)", patch:"$(emerald_ver 3)"}" "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"

# Run oasis-node.
${OASIS_NET_RUNNER} --fixture.file "$FIXTURE_FILE" --basedir "${OASIS_NODE_DATADIR}" --basedir.no_temp_dir
