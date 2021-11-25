package filters

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/event"

	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage"
)

type NewTxsEvent struct{ Txs []*model.Transaction }

type ChainEvent struct {
	Block *model.Block
	Hash  common.Hash
	Logs  []*model.Log
}

type SubscribeAPI interface {
	ChainDB() storage.Storage
	SubscribeNewTxsEvent(chan<- NewTxsEvent) event.Subscription
	SubscribeChainEvent(chan<- ChainEvent) event.Subscription
}

type SubscribeBackend interface {
	SubscribeAPI
	TxChan() chan<- NewTxsEvent
	ChainChan() chan<- ChainEvent
}
