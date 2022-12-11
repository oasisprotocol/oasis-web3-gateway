package filters

import (
	"sync"

	"github.com/ethereum/go-ethereum/event"

	"github.com/oasisprotocol/oasis-web3-gateway/storage"
)

type Subscribe struct {
	err     chan error
	errOnce sync.Once
	chainCh chan<- ChainEvent
	storage storage.Storage
}

func (s *Subscribe) Unsubscribe() {
	s.errOnce.Do(func() {
		close(s.err)
	})
}

func (s *Subscribe) Err() <-chan error {
	return s.err
}

func NewSubscribeBackend(storage storage.Storage) (SubscribeBackend, error) {
	sb := &Subscribe{
		err:     make(chan error, 1),
		storage: storage,
	}
	return sb, nil
}

func (s *Subscribe) SubscribeChainEvent(ch chan<- ChainEvent) event.Subscription {
	s.chainCh = ch
	return s
}

func (s *Subscribe) ChainChan() chan<- ChainEvent {
	return s.chainCh
}
