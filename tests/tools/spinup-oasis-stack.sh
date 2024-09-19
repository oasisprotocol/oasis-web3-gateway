#!/usr/bin/env bash

set -euo pipefail

# This script spins up local oasis node configured with the provided EVM ParaTime.
# Supported ENV Variables:
# - OASIS_NODE_BINARY: path to oasis-node binary
# - OASIS_NET_RUNNER_BINARY: path to oasis-net-runner binary
# - BEACON_BACKEND (optional): choose 'mock' backend (default), or use other behavior
# - PARATIME_BINARY: path to ParaTime binary (inside .orc bundle)
# - PARATIME_VERSION: version of the binary. e.g. 3.0.0
# - ROFL_BINARY (optional): path to ROFL binary (inside .orc bundle)
# - ROFL_BINARY_SGXS (optional): path to signed ROFL binary (inside .orc bundle)
# - OASIS_NODE_DATADIR: path to temporary oasis-node data dir e.g. /tmp/oasis-localnet
# - KEYMANAGER_BINARY (optional): path to key manager binary e.g. simple-keymanager
# - OASIS_SINGLE_COMPUTE_NODE (optional): Only run a single compute node

function paratime_ver {
  echo $PARATIME_VERSION | cut -d \- -f 1 | cut -d + -f 1 | cut -d . -f $1
}
export FIXTURE_FILE="${OASIS_NODE_DATADIR}/fixture.json"
export STAKING_GENESIS_FILE="$(dirname "$0")/staking_genesis.json"

rm -rf "$OASIS_NODE_DATADIR/net-runner"
rm -rf "$OASIS_NODE_DATADIR/net-runner.log"
rm -rf "$OASIS_NODE_DATADIR/fixture.json"
# When $OASIS_NODE_DATADIR is bind-mounted, below fails, but the above succeed
rm -rf "$OASIS_NODE_DATADIR" || true
mkdir -p "$OASIS_NODE_DATADIR" || true

TEE_HARDWARE=""
if [ ! -z "${KEYMANAGER_BINARY:-}" ]; then
  TEE_HARDWARE="intel-sgx"
fi

# Prepare configuration for oasis-node (fixture).
${OASIS_NET_RUNNER_BINARY} dump-fixture \
  --fixture.default.tee_hardware "${TEE_HARDWARE}" \
  --fixture.default.node.binary "${OASIS_NODE_BINARY}" \
  --fixture.default.deterministic_entities \
  --fixture.default.fund_entities \
  --fixture.default.num_entities 2 \
  --fixture.default.keymanager.binary "${KEYMANAGER_BINARY:-}" \
  --fixture.default.runtime.binary "${PARATIME_BINARY}" \
  --fixture.default.runtime.provisioner "unconfined" \
  --fixture.default.runtime.version "$(paratime_ver 1).$(paratime_ver 2).$(paratime_ver 3)" \
  --fixture.default.halt_epoch 100000 \
  --fixture.default.staking_genesis "${STAKING_GENESIS_FILE}" >"$FIXTURE_FILE"

# Determine compute runtime ID.
RT_IDX=0
if [ ! -z "${KEYMANAGER_BINARY:-}" ]; then
  RT_IDX=1
fi

# Mock SGX requires both ELF ("0") and SGX ("1") binaries defined.
if [ ! -z "${KEYMANAGER_BINARY:-}" ]; then
  jq "
    .runtimes[0].deployments[0].components[0].binaries.\"0\" = \"${KEYMANAGER_BINARY}\" |
    .runtimes[${RT_IDX}].deployments[0].components[0].binaries.\"0\" = \"${PARATIME_BINARY}\"
  " "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
  mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"
fi

# Register a ROFL binary, if it exists.
if [ ! -z "${ROFL_BINARY:-}" ] && [ ! -z "${ROFL_BINARY_SGXS:-}" ]; then
  jq "
      .runtimes[${RT_IDX}].deployments[0].components[1].kind = \"rofl\" |
      .runtimes[${RT_IDX}].deployments[0].components[1].binaries.\"0\" = \"${ROFL_BINARY}\" |
      .runtimes[${RT_IDX}].deployments[0].components[1].binaries.\"1\" = \"${ROFL_BINARY_SGXS}\"
    " "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
  mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"
fi

# Use only one compute node.
if [[ ! -z "${OASIS_SINGLE_COMPUTE_NODE:-}" ]]; then
  jq "
    .compute_workers = [.compute_workers[0]] |
    .runtimes[${RT_IDX}].executor.group_size = 1 |
    .runtimes[${RT_IDX}].executor.group_backup_size = 0
  " "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
  mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"
fi

# Enable expensive queries for testing.
jq "
  .clients[0].runtime_config.\"${RT_IDX}\".estimate_gas_by_simulating_contracts = true |
  .clients[0].runtime_config.\"${RT_IDX}\".allowed_queries = [{all_expensive: true}]
" "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"

if [[ ${BEACON_BACKEND-} == 'mock' ]]; then
  # Set beacon backend to 'debug mock'
  jq ".network.beacon.debug_mock_backend = true" "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
  mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"
  jq ".network.beacon.insecure_parameters.interval = 2" "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp"
  mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"
fi

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

jq ".runtimes[${RT_IDX}].txn_scheduler.propose_batch_timeout=2000000000" "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp" # 2 Seconds.
mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"

# Use a batch timeout of 1 second.
jq ".runtimes[${RT_IDX}].txn_scheduler.batch_flush_timeout=1000000000" "$FIXTURE_FILE" >"$FIXTURE_FILE.tmp" # 1 Seconds.
mv "$FIXTURE_FILE.tmp" "$FIXTURE_FILE"

# Run oasis-node.
${OASIS_NET_RUNNER_BINARY} --fixture.file "$FIXTURE_FILE" --basedir "${OASIS_NODE_DATADIR}" --basedir.no_temp_dir --log.format JSON $@
