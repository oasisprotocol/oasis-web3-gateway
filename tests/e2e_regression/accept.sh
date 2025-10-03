#!/bin/bash

# Accept the new actual results as the expected results for the given test suite.

set -euo pipefail

E2E_REGRESSION_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

suite="${1:-}"
if [[ -z "$suite" ]]; then
  echo "Usage: $0 <suite>"
  exit 1
fi

TEST_DIR="$E2E_REGRESSION_DIR/$suite"
if [[ ! -d "$TEST_DIR/actual" ]]; then
  echo "ERROR: No actual results found for suite '$suite'. Run the test first."
  exit 1
fi

# Copy actual results to expected.
echo "Accepting actual results as expected for suite '$suite'..."
rm -rf "$TEST_DIR/expected"
cp -r "$TEST_DIR/actual" "$TEST_DIR/expected"

echo "Done. The new expected results have been saved."
echo "Don't forget to commit the changes to git."
