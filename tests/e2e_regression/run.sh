#!/bin/bash

# This script is a simple e2e regression test for oasis-web3-gateway.
#
# It takes the name of a test suite, and
#  - indexes the block range defined by the suite using e2e_config_1.yaml
#  - runs the gateway with e2e_config_2.yaml
#  - runs a fixed set of Web3 JSON-RPC calls against the gateway
#  - saves the responses to files, then checks that the responses match
#    the expected outputs (from a previous run).
#
# If the differences are expected, simply check the new responses into git.

set -euo pipefail

E2E_REGRESSION_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# Read arg.
suite="${1:-}"
TEST_DIR="$E2E_REGRESSION_DIR/$suite"
if [[ -z "$suite" || ! -e "$TEST_DIR/e2e_config_1.yaml" || ! -e "$TEST_DIR/e2e_config_2.yaml" ]]; then
  cat >&2 <<EOF
Usage: $0 <suite>

Available test suites:
EOF
  for dir in "$E2E_REGRESSION_DIR"/*/; do
    if [[ -f "$dir/e2e_config_1.yaml" && -f "$dir/e2e_config_2.yaml" ]]; then
      echo "  $(basename "$dir")" >&2
    fi
  done
  exit 1
fi

# Clean the database before indexing.
echo "*** Cleaning database..."
./oasis-web3-gateway truncate-db --config "$TEST_DIR/e2e_config_1.yaml" --unsafe

# Index the data.
echo "*** Starting indexing process..."
./oasis-web3-gateway --config "$TEST_DIR/e2e_config_1.yaml"
echo "*** Indexing completed"

# Load test cases.
source "$TEST_DIR/test_cases.sh"

# The hostname of the Web3 gateway to test.
gateway_url="http://localhost:8545"

# The directory to store the actual responses in.
outDir="$TEST_DIR/actual"
mkdir -p "$outDir"
rm "$outDir"/* 2>/dev/null || true

nCases=${#testCases[@]}

# Start the Web3 gateway.
echo "*** Starting Web3 gateway..."
./oasis-web3-gateway --config="${TEST_DIR}/e2e_config_2.yaml" &
gateway_pid=$!

# Kill the gateway on exit.
trap "kill $gateway_pid 2>/dev/null || true; wait $gateway_pid 2>/dev/null || true" SIGINT SIGTERM EXIT

# Wait for gateway to start.
while ! curl --silent --fail "$gateway_url" -X POST -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"web3_clientVersion","params":[],"id":1}' >/dev/null 2>&1; do
  echo "Waiting for Web3 gateway to start..."
  sleep 1
done

echo "*** Web3 gateway started, running test cases..."

# Run the test cases.
seen=("placeholder") # avoids 'seen[*]: unbound variable' error on zsh.
for ((i = 0; i < nCases; i++)); do
  name="$(echo "${testCases[$i]}" | cut -d' ' -f1)"
  rpc_call="$(echo "${testCases[$i]}" | cut -d' ' -f2-)"

  # Sanity check: testcase name should be unique.
  if [[ " ${seen[*]} " =~ " ${name} " ]]; then
    echo "ERROR: test case $name is not unique"
    exit 1
  fi
  seen+=("$name")

  echo "Running test case: $name"

  # Make the JSON-RPC call.
  curl --silent --show-error --dump-header "$outDir/$name.headers" \
    -X POST \
    -H "Content-Type: application/json" \
    -d "$rpc_call" \
    "$gateway_url" > "$outDir/$name.body"

  # Pretty-print JSON for stable diffs.
  if jq empty "$outDir/$name.body" 2>/dev/null; then
    jq '.' < "$outDir/$name.body" > /tmp/pretty 2>/dev/null &&
    cp /tmp/pretty "$outDir/$name.body"
  fi

  # Sanitize version strings for stable diffs.
  if [[ "$name" == "web3_clientVersion" ]]; then
    # Replace the version string with a stable placeholder.
    # Example: "oasis/v5.3.4-git6d9a67a+dirty/go1.24.4" -> "oasis/VERSION"
    # Example: "oasis/-git3baed03/go1.24.7" -> "oasis/VERSION"
    sed -i -E 's|"oasis/[^"]+"|"oasis/VERSION"|g' "$outDir/$name.body"
  fi

  # Sanitize headers for stable diffs.
  if [[ -f "$outDir/$name.headers" ]]; then
    sed -i -E 's/^(Date|Content-Length): .*/\1: UNINTERESTING/g' "$outDir/$name.headers"
  fi
done

# Compare with expected results.
if ! diff --recursive "$TEST_DIR/expected" "$outDir" >/dev/null 2>&1; then
  echo
  echo "NOTE: $TEST_DIR/expected and $outDir differ."

  if [[ -t 1 ]]; then # Running in a terminal.
    echo "Press enter to see the diff, or Ctrl-C to abort."
    read -r
  else
    echo "CI diff:"
  fi

  # Show the diff.
  git diff --no-index "$TEST_DIR/expected" "$outDir" || true

  if [[ -t 1 ]]; then # Running in a terminal.
    echo
    echo "To re-view the diff, run:"
    echo "  git diff --no-index $TEST_DIR/expected $outDir"
    echo
    echo "If the new results are expected, copy the new results into .../expected:"
    echo "  $E2E_REGRESSION_DIR/accept.sh $suite"
  fi
  exit 1
fi

echo
echo "E2E regression test suite \"$suite\" passed!"
