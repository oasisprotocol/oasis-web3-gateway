//May be we will move this file to rpc later.
package indexer

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"

	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
)

func (s *Service) GetBlockByHash(blockHash common.Hash) (*block.Block, error) {
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

func (s *Service) getBlockTransactionsByHash(blockHash common.Hash) ([]*types.UnverifiedTransaction, error) {
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

func (s *Service) GetBlockTransactionsCountByHash(blockHash common.Hash) (uint32, error) {
	txs, err := s.getBlockTransactionsByHash(blockHash)
	if err != nil {
		s.Logger.Error("Call getBlockTransactionsByHash error")
		return 0, err
	}

	return uint32(len(txs)), nil
}

func (s *Service) GetBlockNumberByHash(round uint64) (common.Hash, error) {
	return s.backend.QueryBlockHash(round)
}

func (s *Service) getTransactionByIndex(txs []*types.UnverifiedTransaction, index uint32) (*model.Transaction, error) {
	if index >= uint32(len(txs)) {
		return nil, errors.New("out of tx index")
	}
	return s.backend.Decode(txs[index])
}

func (s *Service) GetTransactionByBlockHashAndIndex(blockHash common.Hash, index uint32) (*model.Transaction, error) {
	txs, err := s.getBlockTransactionsByHash(blockHash)
	if err != nil {
		return nil, err
	}

	return s.getTransactionByIndex(txs, index)
}
