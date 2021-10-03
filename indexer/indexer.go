package indexer

import (
	"context"
	"time"

	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-core/go/common/service"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
)

const (
	storageRequestTimeout = 5 * time.Second
	storageRetryTimeout   = 120 * time.Second
)

// Service is an indexer service.
type Service struct {
	service.BaseBackgroundService
	runtimeID common.Namespace
	backend   Backend
	client    client.RuntimeClient

	ctx       context.Context
	cancelCtx context.CancelFunc

	stopCh chan struct{}
}

func (s *Service) worker() {
	defer s.BaseBackgroundService.Stop()

	s.Logger.Info("started indexer for runtime")

	// Start watching blocks.
	blocksCh, blocksSub, err := s.client.WatchBlocks(s.ctx, s.runtimeID)
	if err != nil {
		s.Logger.Error("failed to subscribe to blocks",
			"err", err,
		)
		return
	}
	defer blocksSub.Close()

	for {
		select {
		case <-s.stopCh:
			s.Logger.Info("stop requested, terminating indexer")
			return
		case annBlk := <-blocksCh:
			// New blocks to index.
			blk := annBlk.Block

			var txs []*types.UnverifiedTransaction
			if !blk.Header.IORoot.IsEmpty() {
				txs, err := s.client.GetTransactions(s.ctx, blk.Header.Round)	
				if err != nil {
					s.Logger.Error("can't get transactions through client")
					continue
				}



				if err = s.backend.Index(blk.Header.Round, blk.Header.EncodedHash(), txs); err != nil {
					s.Logger.Error("failed to index",
						"err", err,
						"round", blk.Header.Round,
				)
				continue
			}
		}
	}
}

func (s *Service) Start() error {
	go s.worker()
	return nil
}

func (s *Service) Stop() {
	s.cancelCtx()
	close(s.stopCh)
}

// New creates a new tag indexer service.
func New(
	dataDir string,
	runtimeID common.Namespace
	backendFactory BackendFactory,
	client    client.RuntimeClient,
) (*Service, error) {
	backend, err := backendFactory(dataDir, runtimeID)
	if err != nil {
		return nil, err
	}

	ctx, cancelCtx := context.WithCancel(context.Background())

	s := &Service{
		BaseBackgroundService: *service.NewBaseBackgroundService("gateway/indexer"),
		runtimeID:             runtimeID,
		backend:               backend,
		client:    			   client,
		ctx:                   ctx,
		cancelCtx:             cancelCtx,
		stopCh:                make(chan struct{}),
	}
	s.Logger = s.Logger.With("runtime_id", s.runtimeID.String())

	return s, nil
}