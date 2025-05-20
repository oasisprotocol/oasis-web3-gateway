#!/usr/bin/env bash

# This script spins up local oasis stack by running the spinup-oasis-stack.sh,
# starts the web3 gateway on top of it, and deposits to a test account on the
# ParaTime.
# Supported ENV Variables:
# - all ENV variables required by spinup-oasis-stack.sh
# - OASIS_WEB3_GATEWAY_BINARY: path to oasis-web3-gateway binary
# - OASIS_WEB3_GATEWAY_CONFIG_FILE: path to oasis-web3-gateway config file
# - BEACON_BACKEND: beacon epoch transition mode 'mock' (default) or 'default'
# - OASIS_SINGLE_COMPUTE_NODE (default: true): if non-empty only run a single compute node
# - OASIS_CLI_BINARY (optional): path to oasis binary. If provided, Oasis CLI will be configured for Localnet
# - ENVOY_BINARY (optional): path to Envoy binary. If provided, Envoy proxy to Oasis Client node will be started
# - ENVOY_CONFIG_FILE: path to Envoy config file. Required if ENVOY_BINARY is provided
# - OASIS_NEXUS_BINARY: path to oasis-nexus binary
# - OASIS_NEXUS_CONFIG_FILE: path to oasis-nexus config file. Required if OASIS_NEXUS_BINARY is provided
# - OASIS_EXPLORER_DIR: path to explorer (nexus frontend) directory
# - OASIS_EXPLORER_NGINX_CONFIG_FILE: path to explorer nginx config file.

rm -f /CONTAINER_READY

export OASIS_DOCKER_START_EXPLORER=${OASIS_DOCKER_START_EXPLORER:-yes}

export OASIS_DOCKER_NO_GATEWAY=${OASIS_DOCKER_NO_GATEWAY:-no}

export OASIS_NODE_LOG_LEVEL=${OASIS_NODE_LOG_LEVEL:-warn}

export OASIS_DOCKER_USE_TIMESTAMPS_IN_NOTICES=${OASIS_DOCKER_USE_TIMESTAMPS_IN_NOTICES:-no}

export OASIS_DOCKER_DEBUG_DISK_AND_CPU_USAGE=${OASIS_DOCKER_DEBUG_DISK_AND_CPU_USAGE:-no}

export OASIS_SINGLE_COMPUTE_NODE=${OASIS_SINGLE_COMPUTE_NODE:-1}

export EXPLORER_PORT=${EXPLORER_PORT:-8548}

OASIS_WEB3_GATEWAY_VERSION=$(${OASIS_WEB3_GATEWAY_BINARY} -v | head -n1 | cut -d " " -f 3 | sed -r 's/^v//')
OASIS_CORE_VERSION=$(${OASIS_NODE_BINARY} -v | head -n1 | cut -d " " -f 3 | sed -r 's/^v//')
VERSION=$(cat /VERSION)

echo "${PARATIME_NAME}-localnet ${VERSION} (oasis-core: ${OASIS_CORE_VERSION}, ${PARATIME_NAME}-paratime: ${PARATIME_VERSION}, oasis-web3-gateway: ${OASIS_WEB3_GATEWAY_VERSION})"
echo

export BEACON_BACKEND=${BEACON_BACKEND:-mock}

OASIS_NODE_SOCKET=${OASIS_NODE_DATADIR}/net-runner/network/client-0/internal.sock
OASIS_KM_SOCKET=${OASIS_NODE_DATADIR}/net-runner/network/keymanager-0/internal.sock

OASIS_WEB3_GATEWAY_PID=""
OASIS_NODE_PID=""
ENVOY_PID=""
NEXUS_PID=""
EXPLORER_PID=""

set -euo pipefail

function cleanup {
  if [[ -n "${OASIS_WEB3_GATEWAY_PID}" ]]; then
    kill -9 ${OASIS_WEB3_GATEWAY_PID}
  fi
  if [[ -n "${OASIS_NODE_PID}" ]]; then
    kill -9 ${OASIS_NODE_PID}
  fi
  if [[ -n "${ENVOY_PID}" ]]; then
    kill -9 ${ENVOY_PID}
  fi
  if [[ -n "${NEXUS_PID}" ]]; then
    kill -9 ${NEXUS_PID}
  fi
  if [[ -n "${EXPLORER_PID}" ]]; then
    kill -9 ${EXPLORER_PID}
  fi
}

trap cleanup INT TERM EXIT

# If we have an interactive terminal, use colors.
if [[ -t 0 ]]; then
  GREEN="\e[32;1m"
  YELLOW="\e[33;1m"
  CYAN="\e[36;1m"
  OFF="\e[0m"
else
  GREEN=""
  YELLOW=""
  CYAN=""
  OFF=""
fi

if [[ "x${OASIS_DOCKER_USE_TIMESTAMPS_IN_NOTICES}" == "xyes" ]]; then
  function notice {
    printf "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${OFF} $@"
  }
  else
  function notice {
    printf "${CYAN} *${OFF} $@"
  }
fi

if [[ "${OASIS_NODE_LOG_LEVEL}" == "debug" ]]; then
  if [[ "x${OASIS_DOCKER_USE_TIMESTAMPS_IN_NOTICES}" == "xyes" ]]; then
  function notice_debug {
    if [ "$1" == "-l" ]; then
      shift
      printf "\n"
    fi
    printf "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${OFF} $@"
  }
  else
  function notice_debug {
    if [ "$1" == "-l" ]; then
      shift
      printf "\n"
    fi
    printf "${CYAN} *${OFF} $@"
  }
  fi
else
  notice_debug() {
    :
  }
fi

# Parse command-line arguments for test account population at the beginning,
# so that we exit early if there are any errors.
AMOUNT="10_000"
TO=""
N="5"
while [[ $# -gt 0 ]]; do
  case $1 in
    -amount)
      # Amount to deposit in ParaTime base units.
      AMOUNT="$2"
      shift
      shift
      ;;
    -to)
      # Comma-separated deposit addresses (0x or oasis1 format) or mnemonic.
      # If empty, a new mnemonic will be generated.
      TO="$2"
      shift
      shift
      ;;
    -test-mnemonic)
      # Use the standard test mnemonic.
      TO="test test test test test test test test test test test junk"
      shift
      ;;
    -n)
      # Number of addresses to derive from mnemonic.
      N="$2"
      shift
      shift
      ;;
    --no-explorer)
      # Don't start explorer and indexer.
      OASIS_DOCKER_START_EXPLORER="no"
      shift
      ;;
    --no-gateway)
      # Don't start web3 gateway.
      OASIS_DOCKER_NO_GATEWAY="yes"
      shift
      ;;
    *)
      echo "Unknown argument: $1"
      exit 1;
      ;;
  esac
done

function populate_accounts() {
  # Remove underscores from amount, as those are only there to make it easier
  # for humans to parse the amount.
  AMOUNT=$(echo "${AMOUNT}" | tr -d '_')

  # If no mnemonic specified, use the test mnemonic.
  if [[ -z ${TO} ]]; then
    TO="test test test test test test test test test test test junk"
 fi

  if [[ ${TO} == *,* ]]; then
    # List of addresses provided, fund these.
    IFS=","
    for addr in ${TO};
    do
      ${OASIS_CLI_BINARY} account deposit ${AMOUNT} ${addr} --account test:alice --gas-price 0 -y > /dev/null
    done
    IFS=" "
  elif [[ ${TO} =~ [[:space:]] ]]; then
    # Mnemonic provided, generate N derivative accounts and fund them.
    for n in $(seq 0 $((N-1)))
    do
      acct="account$n"
      ${OASIS_CLI_BINARY} wallet import ${acct} --algorithm secp256k1-bip44 --number ${n} --secret "${TO}" -y > /dev/null
      ${OASIS_CLI_BINARY} account deposit ${AMOUNT} ${acct} --account test:alice --gas-price 0 -y > /dev/null
    done

    ACCT_OUT=""
    PK_OUT=""
    for n in $(seq 0 $((N-1)))
    do
      acct="account$n"

      # Parse Ethereum address and private key from output.
      IFS="#"
      out=$(${OASIS_CLI_BINARY} wallet export ${acct} -y)
      eth_addr=$(echo "${out}" | grep -e '^Ethereum address:' | cut -d':' -f2 | awk '{$1=$1};1')
      pk=$(echo "${out}" | tail -1)
      IFS=" "

      if [[ $n != 0 ]]; then
        ACCT_OUT="${ACCT_OUT} "
        PK_OUT="${PK_OUT} "
      fi
      ACCT_OUT="${ACCT_OUT}${eth_addr}"
      PK_OUT="${PK_OUT}${pk}"
    done

    echo "Available Accounts"
    echo "=================="
    i=0
    for a in ${ACCT_OUT};
    do
      echo "($i) $a (${AMOUNT} TEST)"
      i=$((i+1))
    done

    echo
    echo "Private Keys"
    echo "=================="
    i=0
    for pk in ${PK_OUT};
    do
      echo "($i) 0x$pk"
      i=$((i+1))
    done

    echo
    echo "HD Wallet"
    echo "=================="
    echo "Mnemonic:     ${TO}"
    echo "Base HD Path: m/44'/60'/0'/0/%d"
  fi
}

T_START="$(date +%s)"

if [[ "${PARATIME_NAME}" == "sapphire" ]]; then
  ROFLS=($(ls /rofls/*.orc 2>/dev/null || exit 0))
  if [[ ${#ROFLS[@]} -eq 0 ]]; then
    notice "No ROFLs detected.\n"
  else
    unzip -q "${ROFLS[0]}" "app.elf"
    mv "app.elf" "rofl.elf"
    # Create a dummy, non-empty SGXS for mock sgx.
    echo "dummy" > "rofl.sgxs"
    export ROFL_BINARY="/rofl.elf"
    export ROFL_BINARY_SGXS="/rofl.sgxs"
    notice "Detected ROFL bundle: ${ROFLS[0]}\n"
  fi
fi

notice "Starting oasis-net-runner with ${CYAN}${PARATIME_NAME}${OFF}...\n"
/spinup-oasis-stack.sh --log.level ${OASIS_NODE_LOG_LEVEL} 2>1 &>/var/log/spinup-oasis-stack.log &
OASIS_NODE_PID=$!

notice "Waiting for Postgres to start"
su -c "POSTGRES_USER=postgres POSTGRES_PASSWORD=postgres /usr/local/bin/docker-entrypoint.sh postgres &" postgres 2>1 &>/var/log/postgres.log
function is_postgres_ready() {
  pg_isready -h 127.0.0.1 -p 5432 2>1 &>/dev/null
}
until is_postgres_ready; do echo -n .; sleep 1; done
echo

notice "Waiting for Oasis node to start..."
# Wait for oasis-node socket before starting web3 gateway.
while [[ ! -S ${OASIS_NODE_SOCKET} ]]; do echo -n .; sleep 1; done
echo

if [ ! -z "${ENVOY_BINARY:-}" ]; then
  notice "Waiting for Envoy proxy to start"

  ${ENVOY_BINARY} -c /envoy.yml 2>1 &>/var/log/envoy.log &
  ENVOY_PID=$!

  # Wait for Envoy to start.
  while ! curl -s http://localhost:8544/ 2>1 &>/dev/null; do echo -n .; sleep 1; done
  echo
fi

# Fixes permissions so oasis-web3-gateway tests can be run
# While /serverdir/node is bind-mounted to /tmp/eth-runtime-test
chmod 755 /serverdir/node/net-runner/
chmod 755 /serverdir/node/net-runner/network/
chmod 755 /serverdir/node/net-runner/network/client-0/
chmod a+rw /serverdir/node/net-runner/network/client-0/internal.sock

if [[ "x${OASIS_DOCKER_NO_GATEWAY}" == "xyes" ]]; then
  notice "Skipping oasis-web3-gateway start-up...\n"
else
  notice "Starting oasis-web3-gateway...\n"
  LOG__LEVEL=${OASIS_NODE_LOG_LEVEL} ${OASIS_WEB3_GATEWAY_BINARY} --config ${OASIS_WEB3_GATEWAY_CONFIG_FILE} 2>1 &>/var/log/oasis-web3-gateway.log &
  OASIS_WEB3_GATEWAY_PID=$!
fi

# Wait for compute nodes before initiating deposit.
notice "Bootstrapping network (this might take a minute)"
if [[ "${BEACON_BACKEND}" == "mock" ]]; then
  echo -n .
  notice_debug -l "Waiting for nodes to be ready..."
  ${OASIS_NODE_BINARY} debug control wait-nodes -n 2 -a unix:${OASIS_NODE_SOCKET}

  if [[ "${PARATIME_NAME}" == "sapphire" ]]; then
    echo -n .
    # The key manager registers before it is fully ready, so the previous 'wait-nodes' check is not sufficient.
    # Ensure the key manager is fully ready before moving to the first epoch.
    notice_debug -l "Waiting for key manager to start up..."
    ${OASIS_NODE_BINARY} debug control wait-ready -a unix:${OASIS_KM_SOCKET}

    # Store the keymanager's runtime extra_info to monitor for changes during epoch 1.
    km_extra_info=$(${OASIS_NODE_BINARY} control status -a unix:${OASIS_KM_SOCKET} | jq -r '.registration.descriptor.runtimes[0].extra_info')
  fi

  echo -n .
  notice_debug -l "Setting epoch to 1..."
  ${OASIS_NODE_BINARY} debug control set-epoch --epoch 1 -a unix:${OASIS_NODE_SOCKET}

  if [[ "${PARATIME_NAME}" == "sapphire" ]]; then
    # Ensure ephemeral secret has been generated.
    notice_debug -l "Waiting for key manager to generate ephemeral secret..."
    while (${OASIS_NODE_BINARY} control status -a unix:${OASIS_KM_SOCKET} | jq -e ".keymanager.secrets.worker.ephemeral_secrets.last_generated_epoch!=2" >/dev/null); do
      sleep 0.5
    done

    # Ensure the keymanager has re-registered with updated extra_info.
    # This ensures that we wait until the master secret has been proposed and confirmed.
    notice_debug -l "Waiting for key manager to generate master secret and re-register..."
    while [[ $(${OASIS_NODE_BINARY} control status -a unix:${OASIS_KM_SOCKET} | jq -r '.registration.descriptor.runtimes[0].extra_info') == "${km_extra_info}" ]]; do
      sleep 0.5
    done
  fi

  echo -n .
  notice_debug -l "Setting epoch to 2..."
  ${OASIS_NODE_BINARY} debug control set-epoch --epoch 2 -a unix:${OASIS_NODE_SOCKET}

else
  echo -n ...
  notice_debug -l "Waiting for nodes to be ready..."
  ${OASIS_NODE_BINARY} debug control wait-ready -a unix:${OASIS_NODE_SOCKET}
fi
echo

if [[ "x${OASIS_DOCKER_NO_GATEWAY}" != "xyes" && "${PARATIME_NAME}" == "sapphire" ]]; then
  notice "Waiting for key manager..."
  function is_km_ready() {
    curl -X POST -s \
      -H 'Content-Type: application/json' \
      --data '{"jsonrpc":"2.0","method":"oasis_callDataPublicKey","params":[],"id":1}' \
      http://127.0.0.1:8545 2>1 | jq -e '.result | has("key")' 2>1 &>/dev/null
  }
  until is_km_ready; do
    echo -n .
    sleep 1
  done
  echo
else
  # Wait a bit to ensure client is ready if we're not waiting for keymanager.
  sleep 10
fi

# Once everything is initialized and setup, start Nexus and Explorer if enabled.
if [[ "${OASIS_DOCKER_START_EXPLORER}" == "yes" ]]; then
  notice "Creating database 'nexus'\n"
  su -c "createdb -h 127.0.0.1 -p 5432 -U postgres nexus" postgres

  notice "Waiting for Nexus to start"
  # Configure Nexus config file.
  chain_context=$(${OASIS_NODE_BINARY} control status -a unix:${OASIS_NODE_SOCKET} | jq -r .consensus.chain_context)
  sed -i 's/{{CHAIN_CONTEXT}}/'"${chain_context}"'/g' ${OASIS_NEXUS_CONFIG_FILE}
  sed -i 's/{{LOG_LEVEL}}/'"${OASIS_NODE_LOG_LEVEL}"'/g' ${OASIS_NEXUS_CONFIG_FILE}
  sed -i 's/{{PARATIME_NAME}}/'"${PARATIME_NAME}"'/g' ${OASIS_NEXUS_CONFIG_FILE}

  ${OASIS_NEXUS_BINARY} --config ${OASIS_NEXUS_CONFIG_FILE} 2>1 &>/var/log/nexus.log &
  NEXUS_PID=$!

  # Wait for Oasis Nexus to start.
  while ! curl -s http://localhost:8547/ 2>1 &>/dev/null; do echo -n .; sleep 1; done
  echo

  notice "Waiting for Explorer to start"
  # Configure the nginx config file.
  sed -i 's|{{EXPLORER_PORT}}|'"${EXPLORER_PORT}"'|g' "${OASIS_EXPLORER_NGINX_CONFIG_FILE}"
  sed -i 's|{{EXPLORER_DIR}}|'"${OASIS_EXPLORER_DIR}"'|g' "${OASIS_EXPLORER_NGINX_CONFIG_FILE}"
  ln -s ${OASIS_EXPLORER_NGINX_CONFIG_FILE} /etc/nginx/sites-enabled/explorer
  service nginx start 2>1 &>/dev/null
  nginx -s reload 2>1 &>/dev/null

  # Wait for Explorer to start.
  while ! curl -s http://localhost:${EXPLORER_PORT}/ 2>1 &>/dev/null; do echo -n .; sleep 1; done
  echo
fi

# Add Localnet to Oasis CLI and make it default.
if [ ! -z "${OASIS_CLI_BINARY:-}" ]; then
  # XXX: Fix to absolute socket path once https://github.com/oasisprotocol/cli/issues/471 is fixed.
  ${OASIS_CLI_BINARY} network add-local localnet unix:serverdir/node/net-runner/network/client-0/internal.sock -y
  ${OASIS_CLI_BINARY} paratime add localnet ${PARATIME_NAME} 8000000000000000000000000000000000000000000000000000000000000000 --num-decimals 18 -y
  ${OASIS_CLI_BINARY} network set-default localnet
fi


notice "Populating accounts...\n\n"
populate_accounts


# Register ROFL and fund accounts.
if [ ! -z "${ROFL_BINARY:-}" ]; then
  # Since rofl.sgxs and MR_SIGNER never change, we can hardcode enclave identity.
  # !/usr/bin/python
  # import codecs
  # mr_enclave = b'01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805daca546b' # sha256sum /rofl.sgxs
  # mr_signer = b'9affcfae47b848ec2caf1c49b4b283531e1cc425f93582b36806e52a43d78d1a'  # hardcoded test MR_SIGNER
  # codecs.encode(codecs.decode(mr_enclave + mr_signer, 'hex'), 'base64').decode().replace('\n', '')
  ROFL_ENCLAVE_ID="0+tTmlVjUvP0eIHXH7Dld3svPppCUdKDwYxnzplndLea/8+uR7hI7CyvHEm0soNTHhzEJfk1grNoBuUqQ9eNGg=="
  ROFL_ADMIN="test:bob"
  ROFL_ADMIN_FUND=10001
  COMPUTE_NODE_ADDRESS="oasis1qp6tl30ljsrrqnw2awxxu2mtxk0qxyy2nymtsy90"
  COMPUTE_NODE_FUND=1000
  POLICY_PATH=/policy-localnet.yml

  echo
  notice "Configuring ROFL ${ROFLS[0]}:\n"
  printf "   Enclave ID: ${CYAN}${ROFL_ENCLAVE_ID}${OFF}\n"

  ${OASIS_CLI_BINARY} account deposit ${ROFL_ADMIN_FUND} ${ROFL_ADMIN} --account test:alice --gas-price 0 -y >/dev/null
  printf "   ROFL admin ${CYAN}${ROFL_ADMIN}${OFF} funded ${ROFL_ADMIN_FUND} TEST\n"

  ${OASIS_CLI_BINARY} account deposit ${COMPUTE_NODE_FUND} ${COMPUTE_NODE_ADDRESS} --account test:alice --gas-price 0 -y >/dev/null
  printf "   Compute node ${CYAN}${COMPUTE_NODE_ADDRESS}${OFF} funded ${COMPUTE_NODE_FUND} TEST\n"

  cat > ${POLICY_PATH} << EOF
quotes:
  pcs:
    tcb_validity_period: 30
    min_tcb_evaluation_data_number: 16
enclaves:
  - "${ROFL_ENCLAVE_ID}"
endorsements:
  - any: {}
fees: endorsing_node
max_expiration: 3
EOF
  # XXX: Report ROFL app ID in JSON and properly parse it.
  ROFL_APP_ID=$(${OASIS_CLI_BINARY} rofl create ${POLICY_PATH} --account ${ROFL_ADMIN} --scheme cn -y | tail -n1 | rev | cut -d' ' -f1 | rev)
  printf "   App ID: ${CYAN}${ROFL_APP_ID}${OFF}\n"
fi

T_END="$(date +%s)"

echo
printf "${YELLOW}WARNING: The chain is running in ephemeral mode. State will be lost after restart!${OFF}\n\n"
notice "GRPC listening on ${CYAN}http://localhost:8544${OFF}.\n"

if [[ "${OASIS_DOCKER_NO_GATEWAY}" == "no" ]]; then
  notice "Web3 RPC listening on ${CYAN}http://localhost:8545${OFF} and ${CYAN}ws://localhost:8546${OFF}. Chain ID: ${GATEWAY__CHAIN_ID}.\n"
fi
if [[ "${OASIS_DOCKER_START_EXPLORER}" == "yes" ]]; then
  notice "Nexus API listening on ${CYAN}http://localhost:8547${OFF}.\n"
  notice "Localnet Explorer available at ${CYAN}http://localhost:${EXPLORER_PORT}${OFF}.\n"
fi

notice "Container start-up took ${CYAN}$((T_END-T_START))${OFF} seconds, node log level is set to ${CYAN}${OASIS_NODE_LOG_LEVEL}${OFF}.\n"

touch /CONTAINER_READY

if [[ "x${OASIS_DOCKER_DEBUG_DISK_AND_CPU_USAGE}" == "xyes" ]]; then
  # When debugging disk and CPU usage, we terminate the container
  # after 60 minutes.
  QUIT_AFTER_N_REPORTS=6
  NUM_REPORTS=0

  # Also print a header for the readings below.
  echo
  echo "Uptime [minutes],Total node dir size [kB],Size of node logs [kB],Percent usage by logs [%],Load average (1 min),Load average (5 min),Load average (15 min)"
fi

if [[ "${BEACON_BACKEND}" == "mock" ]]; then
  # Run background task to switch epochs every 10 minutes.
  while true; do
    if [[ "x${OASIS_DOCKER_DEBUG_DISK_AND_CPU_USAGE}" == "xyes" ]]; then
      LOADAVG=$(uptime | cut -d':' -f5 | tr -d ' ')
      TOTAL_NODE_DIR_SIZE_KB=$(du -sk /serverdir/node | cut -f1)
      TOTAL_NODE_LOGS_SIZE_KB=$(du -skc /serverdir/node/net-runner/network/*/*.log | tail -1 | cut -f1)
      LOGS_PCT=$(echo "scale=4; (${TOTAL_NODE_LOGS_SIZE_KB} / ${TOTAL_NODE_DIR_SIZE_KB}) * 100" | bc)

      echo "$((NUM_REPORTS * 10)),${TOTAL_NODE_DIR_SIZE_KB},${TOTAL_NODE_LOGS_SIZE_KB},${LOGS_PCT},${LOADAVG}"

      if [[ ${NUM_REPORTS} -ge ${QUIT_AFTER_N_REPORTS} ]]; then
        exit 0
      fi
      NUM_REPORTS=$((NUM_REPORTS + 1))
    fi

    sleep $((60*10))

    epoch=`${OASIS_NODE_BINARY} control status -a unix:${OASIS_NODE_SOCKET} | jq '.consensus.latest_epoch'`
    epoch=$((epoch + 1))
    ${OASIS_NODE_BINARY} debug control set-epoch --epoch $epoch -a unix:${OASIS_NODE_SOCKET}
  done
else
  wait
fi
