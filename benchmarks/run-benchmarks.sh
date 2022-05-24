#!/usr/bin/env bash
set -o nounset -o pipefail -o errexit -x

# Kill all dangling processes on exit.
cleanup() {
	pkill -P $$ || true
	wait || true
}
trap "cleanup" EXIT

TEST_BASE_DIR="/tmp/oasis-emerald-benchmarks"
CLIENT_SOCK="unix:${TEST_BASE_DIR}/net-runner/network/client-0/internal.sock"

# Generate fixture.
mkdir -p "${TEST_BASE_DIR}"
./benchmarks \
	fixture \
	--node.binary "${OASIS_NODE}" \
	--runtime.id "8000000000000000000000000000000000000000000000000000000000000000" \
	--runtime.version "${EMERALD_PARATIME_VERSION}" \
	--runtime.binary "${EMERALD_PARATIME}" >"${TEST_BASE_DIR}/fixture.json"

# Start the network.
"${OASIS_NET_RUNNER}" \
	--fixture.file "${TEST_BASE_DIR}/fixture.json" \
	--basedir "${TEST_BASE_DIR}" \
	--basedir.no_temp_dir &

sleep 10

"${EMERALD_WEB3_GATEWAY}" --config ../conf/benchmarks.yml &

./benchmarks \
	--address "${CLIENT_SOCK}" \
	--wait \
	--runtime.id "8000000000000000000000000000000000000000000000000000000000000000" \
	--runtime.chain_id "${GATEWAY__CHAIN_ID}" \
	--gateway_url "http://localhost:8545" \
	--benchmarks evm_transfer,evm_blockNumber,evm_balance \
	--benchmarks.concurrency 300
