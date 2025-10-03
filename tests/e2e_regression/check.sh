#!/bin/bash

# Check actual vs expected results for a given test suite without running the tests.
# This is useful for reviewing differences after a test run.

set -euo pipefail

E2E_REGRESSION_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# Read arg.
suite="${1:-}"
if [[ -z "$suite" ]]; then
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

TEST_DIR="$E2E_REGRESSION_DIR/$suite"
if [[ ! -d "$TEST_DIR" ]]; then
  echo "ERROR: Test suite '$suite' not found."
  exit 1
fi

outDir="$TEST_DIR/actual"
expectedDir="$TEST_DIR/expected"

if [[ ! -d "$outDir" ]]; then
  echo "ERROR: No actual results found for suite '$suite'. Run the test first:"
  echo "  ./tests/e2e_regression/run.sh $suite"
  exit 1
fi

if [[ ! -d "$expectedDir" ]]; then
  echo "ERROR: No expected results found for suite '$suite'."
  echo "If this is the first run, accept the actual results as expected:"
  echo "  ./tests/e2e_regression/accept.sh $suite"
  exit 1
fi

# Compare actual vs expected results.
if diff --recursive "$expectedDir" "$outDir" >/dev/null 2>&1; then
  echo "✓ Test suite '$suite' results match expected outputs."
  exit 0
fi

echo "✗ Test suite '$suite' results differ from expected outputs."
echo

if [[ -t 1 ]]; then # Running in a terminal.
  echo "Press enter to see the diff, or Ctrl-C to abort."
  read -r
else
  echo "Differences:"
fi

# Show the diff using git diff for better formatting.
git diff --no-index "$expectedDir" "$outDir" || true

if [[ -t 1 ]]; then # Running in a terminal.
  echo
  echo "To re-view the diff, run:"
  echo "  git diff --no-index $expectedDir $outDir"
  echo
  echo "If the new results are expected, accept them:"
  echo "  ./tests/e2e_regression/accept.sh $suite"
fi

exit 1
