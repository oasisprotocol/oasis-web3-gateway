#!/usr/bin/env bash

set -euo pipefail

TAG=${TAG:-ghcr.io/oasisprotocol/emerald-localnet:local}
NAME="emerald-localnet-test"

cleanup() {
	# Print standard output content.
	docker logs "${NAME}" || true
	# Stop the docker container.
	docker stop "${NAME}" >/dev/null || true
}

trap cleanup INT TERM

docker run -itd --rm -p8545:8545 --name "${NAME}" "${TAG}" -test-mnemonic >/dev/null

# Check, if depositing tokens to test accounts worked.
while true; do
	OUT=$(curl -s -H "Content-Type: application/json" -X POST --data '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0x15d34AAf54267DB7D7c367839AAf71A00a2C6A65", "latest"],"id":1}' http://localhost:8545 || true)
	echo $OUT | grep -q 0x21e19e0c9bab2400000 && break
	sleep 1
done

cleanup
