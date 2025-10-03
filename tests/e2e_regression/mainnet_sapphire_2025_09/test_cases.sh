#!/bin/bash

# This file holds test cases for the mainnet_sapphire_2025_09 e2e regression test.
#
# Each test case is a pair of (name, JSON-RPC call).
# These are suite-specific test cases that depend on the indexed block range
# (10840395-10842395) or other suite-specific parameters.

# Load common test cases.
source "$(dirname "${BASH_SOURCE[0]}")/../common_test_cases.sh"

suiteSpecificTestCases=(
  # Block-related methods with specific block numbers.
  'eth_getBlockByNumber_10840400               {"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0xa56950",false],"id":1}'
  'eth_getBlockByNumber_10840400_full          {"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0xa56950",true],"id":1}'
  'eth_getBlockByNumber_10841395               {"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0xa56d33",false],"id":1}'
  'eth_getBlockByNumber_out_of_range           {"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0xa5c018",false],"id":1}'
  'eth_getBlockByHash_10840400                 {"jsonrpc":"2.0","method":"eth_getBlockByHash","params":["0x527e92af5c99b9cedae3c9a23e79a90411d348780c15004c8ef6a3ad2e20c472",false],"id":1}'
  'eth_getBlockByHash_10841938                 {"jsonrpc":"2.0","method":"eth_getBlockByHash","params":["0x30b477ba19fa5481728bb502c57f7109634621e3a9c9f63893c387874cc2a8bd",false],"id":1}'
  'eth_getBlockHash_10840400                   {"jsonrpc":"2.0","method":"eth_getBlockHash","params":["0xa56950",false],"id":1}'

  # Block transaction counts with specific blocks.
  'eth_getBlockTransactionCountByHash_10840400          {"jsonrpc":"2.0","method":"eth_getBlockTransactionCountByHash","params":["0x527e92af5c99b9cedae3c9a23e79a90411d348780c15004c8ef6a3ad2e20c472"],"id":1}'
  'eth_getBlockTransactionCountByNumber_10840400        {"jsonrpc":"2.0","method":"eth_getBlockTransactionCountByNumber","params":["0xa56950"],"id":1}'
  'eth_getBlockTransactionCountByHash_10841938          {"jsonrpc":"2.0","method":"eth_getBlockTransactionCountByHash","params":["0x30b477ba19fa5481728bb502c57f7109634621e3a9c9f63893c387874cc2a8bd"],"id":1}'
  'eth_getBlockTransactionCountByNumber_10841938        {"jsonrpc":"2.0","method":"eth_getBlockTransactionCountByNumber","params":["0xa56f52"],"id":1}'

  # Transaction receipts and details.
  'eth_getTransactionByHash                    {"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":["0x0161e5250c86504c13a6f2b82d6e832e60843d1d8b63079f455acf4f49bdf73c"],"id":1}'
  'eth_getTransactionReceipt                   {"jsonrpc":"2.0","method":"eth_getTransactionReceipt","params":["0x0161e5250c86504c13a6f2b82d6e832e60843d1d8b63079f455acf4f49bdf73c"],"id":1}'
  'eth_getTransactionByBlockHashAndIndex       {"jsonrpc":"2.0","method":"eth_getTransactionByBlockHashAndIndex","params":["0x30b477ba19fa5481728bb502c57f7109634621e3a9c9f63893c387874cc2a8bd","0x0"],"id":1}'
  'eth_getTransactionByBlockNumberAndIndex     {"jsonrpc":"2.0","method":"eth_getTransactionByBlockNumberAndIndex","params":["0xa56f52","0x0"],"id":1}'

  # Logs and events within the indexed range.
  'eth_getLogs_range_empty                     {"jsonrpc":"2.0","method":"eth_getLogs","params":[{"fromBlock":"0xa56950","toBlock":"0xa56960","topics":[]}],"id":1}'
  'eth_getLogs_range_wider                     {"jsonrpc":"2.0","method":"eth_getLogs","params":[{"fromBlock":"0xa56950","toBlock":"0xa56d33","topics":[]}],"id":1}'

  # State access methods with specific round.
  'eth_getBalance_zero                         {"jsonrpc":"2.0","method":"eth_getBalance","params":["0x0000000000000000000000000000000000000000","0xa56950"],"id":1}'
  'eth_getCode_zero                            {"jsonrpc":"2.0","method":"eth_getCode","params":["0x0000000000000000000000000000000000000000","0xa56950"],"id":1}'
  'eth_getStorageAt_zero                       {"jsonrpc":"2.0","method":"eth_getStorageAt","params":["0x0000000000000000000000000000000000000000","0x0","0xa56950"],"id":1}'
  'eth_getTransactionCount_zero                {"jsonrpc":"2.0","method":"eth_getTransactionCount","params":["0x0000000000000000000000000000000000000000","0xa56950"],"id":1}'

  # Call and gas estimation methods with specific round.
  'eth_call_empty                              {"jsonrpc":"2.0","method":"eth_call","params":[{"to":"0x0000000000000000000000000000000000000000","data":"0x"},"0xa56950"],"id":1}'
  'eth_estimateGas_empty                       {"jsonrpc":"2.0","method":"eth_estimateGas","params":[{"to":"0x0000000000000000000000000000000000000000","data":"0x"},"0xa56950"],"id":1}'
)

# Combine common and suite-specific test cases.
testCases=("${commonTestCases[@]}" "${suiteSpecificTestCases[@]}")
