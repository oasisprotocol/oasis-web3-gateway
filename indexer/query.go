//May be we will move this file to rpc later.
package indexer

import (
	"errors"
	"hash"

	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
)

func (s *Service) GetBlockByHash(blockHash hash.Hash) (*block.Block, error) {
	round, err1 := s.backend.QueryBlockRound(blockHash)
	if err1 != nil {
		s.Logger.Error("Matched block error, block hash: ", blockHash)
		return nil, err1
	}

	blk, err2 := s.client.GetBlock(s.ctx, round)
	if err2 != nil {
		s.Logger.Error("Matched block error, block round: ", round)
		return nil, err2
	}

	return blk, nil
}

func (s *Service) getBlockTransactionsByNumber(round uint64) ([]*types.UnverifiedTransaction, error) {
	blk, err1 := s.client.GetBlock(s.ctx, round)
	if err1 != nil {
		s.Logger.Error("Matched block error, block round: ", round)
		return nil, err1
	}

	txs, err := s.client.GetTransactions(s.ctx, blk.Header.Round)
	if err != nil {
		s.Logger.Error("Call GetTransactions error")
		return nil, err
	}

	return txs, nil
}

func (s *Service) getBlockTransactionsByHash(blockHash hash.Hash) ([]*types.UnverifiedTransaction, error) {
	blk, err := s.GetBlockByHash(blockHash)
	if err != nil {
		s.Logger.Error("Matched block error when call GetBlockByHash, block hash: ", blockHash)
		return nil, err
	}

	txs, err := s.client.GetTransactions(s.ctx, blk.Header.Round)
	if err != nil {
		s.Logger.Error("Call GetTransactions error")
		return nil, err
	}

	return txs, nil
}

func (s *Service) GetBlockTransactionsCountByHash(blockHash hash.Hash) (uint32, error) {
	txs, err := getBlockTransactionsByHash(blockHash)
	if err != nil {
		s.Logger.Error("Call getBlockTransactionsByHash error")
		return 0, err
	}

	return uint32(len(txs)), nil
}

func (s *Service) GetBlockNumberByHash(round uint64) (hash.Hash, error) {
	return s.backend.QueryBlockHash(round)
}

func (s *Service) getTxResultByHash(ethTransactionHash hash.Hash) (*model.TxResult, error) {
	return s.backend.QueryTxResult(ethTransactionHash)
}

type EthTransaction struct {
	//to do
	tx string
}

func (s *Service) getTransactionByIndex(txs []*types.UnverifiedTransaction, index uint32) (*EthTransaction, error) {
	if index >= len(txs) {
		return nil, errors.New("Index is too large")
	}

	innerTx := txs[result.Index]

	//todo: decode oasis tx to eth tx
	var ethTx EthTransaction

	return ethTx, nil
}

func (s *Service) GetTransactionByHash(ethTransactionHash hash.Hash) (*EthTransaction, error) {
	result, err := s.backend.QueryTxResult(ethTransactionHash)
	if err != nil {
		s.Logger.Error("Get transaction result error!")
		return nil, err
	}

	blk, err := s.client.GetBlock(s.ctx, result.Round)
	if err != nil {
		s.Logger.Error("Get block result error!")
		return nil, err
	}

	txs, err := s.client.GetTransactions(s.ctx, blk.Header.Round)
	if err != nil {
		s.Logger.Error("Call GetTransactions error")
		return nil, err
	}

	return getTransactionByIndex(txs, result.Index)
}

func (s *Service) GetTransactionByBlockHashAndIndex(blockHash hash.Hash, index uint32) (*EthTransaction, error) {
	txs, err := getBlockTransactionsByHash(blockHash)
	if err != nil {
		s.Logger.Error("Call getBlockTransactionsByHash error")
		return nil, err
	}

	return getTransactionByIndex(txs, index)
}

func (s *Service) GetTansactionByBlockNumberAndIndex(round uint64, index uint32) (*EthRransaction, error) {
	txs, err := getBlockTransactionsByNumber(round)
	if err != nil {
		s.Logger.Error("Call getBlockTransactionsByNumber error")
		return nil, err
	}

	return getTransactionByIndex(txs, index)
}

func (s *Service) GetTransactionReceipt(hash.Hash) (string, error) {
	//to do
	return "", nil
}
