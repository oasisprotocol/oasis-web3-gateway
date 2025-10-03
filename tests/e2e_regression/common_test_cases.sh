#!/bin/bash

# This file holds common test cases for the e2e_regression tests.
#
# Each test case is a pair of (name, JSON-RPC call).
# These are generic test cases that don't depend on specific block numbers,
# transaction hashes, or other suite-specific parameters.

commonTestCases=(
  # Web3 methods.
  'web3_clientVersion        {"jsonrpc":"2.0","method":"web3_clientVersion","params":[],"id":1}'
  'web3_sha3                 {"jsonrpc":"2.0","method":"web3_sha3","params":["0x68656c6c6f20776f726c64"],"id":1}'

  # Network methods.
  'net_version               {"jsonrpc":"2.0","method":"net_version","params":[],"id":1}'
  'net_listening             {"jsonrpc":"2.0","method":"net_listening","params":[],"id":1}'
  'net_peerCount             {"jsonrpc":"2.0","method":"net_peerCount","params":[],"id":1}'

  # Node info methods.
  'eth_chainId                             {"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'
  'eth_mining                              {"jsonrpc":"2.0","method":"eth_mining","params":[],"id":1}'
  'eth_hashrate                            {"jsonrpc":"2.0","method":"eth_hashrate","params":[],"id":1}'
  'eth_coinbase                            {"jsonrpc":"2.0","method":"eth_coinbase","params":[],"id":1}'
  'eth_protocolVersion                     {"jsonrpc":"2.0","method":"eth_protocolVersion","params":[],"id":1}'
  'eth_syncing                             {"jsonrpc":"2.0","method":"eth_syncing","params":[],"id":1}'
  'eth_accounts                            {"jsonrpc":"2.0","method":"eth_accounts","params":[],"id":1}'

  # Latest state methods.
  'eth_blockNumber                         {"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
  'eth_getBlockTransactionCountByNumber    {"jsonrpc":"2.0","method":"eth_getBlockTransactionCountByNumber","params":["latest"],"id":1}'

  # Gas pricing methods.
  'eth_gasPrice                            {"jsonrpc":"2.0","method":"eth_gasPrice","params":[],"id":1}'
  'eth_maxPriorityFeePerGas                {"jsonrpc":"2.0","method":"eth_maxPriorityFeePerGas","params":[],"id":1}'
  'eth_feeHistory                          {"jsonrpc":"2.0","method":"eth_feeHistory","params":["0x4","latest",[]],"id":1}'

  # Logs methods.
  'eth_getLogs_latest                      {"jsonrpc":"2.0","method":"eth_getLogs","params":[{"fromBlock":"latest","toBlock":"latest"}],"id":1}'

  # Oasis-specific methods.
  'oasis_callDataPublicKey                 {"jsonrpc":"2.0","method":"oasis_callDataPublicKey","params":[],"id":1}'

  # Uncle methods (should return 0/null).
  'eth_getUncleCountByBlockNumber          {"jsonrpc":"2.0","method":"eth_getUncleCountByBlockNumber","params":["latest"],"id":1}'
  'eth_getUncleByBlockNumberAndIndex       {"jsonrpc":"2.0","method":"eth_getUncleByBlockNumberAndIndex","params":["latest","0x0"],"id":1}'
)
