package filters

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/event"

	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
)

type ChainEvent struct {
	Block *model.Block
	Hash  common.Hash
	Logs  []*model.Log
}

type SubscribeAPI interface {
	SubscribeChainEvent(chan<- ChainEvent) event.Subscription
}

type SubscribeBackend interface {
	SubscribeAPI
	ChainChan() chan<- ChainEvent
}
