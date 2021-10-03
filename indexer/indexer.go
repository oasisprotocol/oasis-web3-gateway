package indexer

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/oasisprotocol/oasis-core/go/common"
	cmnBackoff "github.com/oasisprotocol/oasis-core/go/common/backoff"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-core/go/common/service"
	"github.com/oasisprotocol/oasis-core/go/runtime/transaction"
	storage "github.com/oasisprotocol/oasis-core/go/storage/api"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
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
	storageBackend storage.Backend

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

			// Fetch transactions from storage.
			//
			// NOTE: Currently the indexer requires all transactions as well since it needs to
			//       expose a notion of a "transaction index within a block" which is hard to
			//       provide as batches can be merged in arbitrary order and the sequence can
			//       only be known after the fact.
			var txs []*transaction.Transaction
			var tags transaction.Tags
			if !blk.Header.IORoot.IsEmpty() {
				off := cmnBackoff.NewExponentialBackOff()
				off.MaxElapsedTime = storageRetryTimeout

				err = backoff.Retry(func() error {
					bctx, cancel := context.WithTimeout(s.ctx, storageRequestTimeout)
					defer cancel()

					// Prioritize nodes that signed the storage receipt.
					bctx = storage.WithNodePriorityHintFromSignatures(bctx, blk.Header.StorageSignatures)

					ioRoot := storage.Root{
						Namespace: blk.Header.Namespace,
						Version:   blk.Header.Round,
						Type:      storage.RootTypeIO,
						Hash:      blk.Header.IORoot,
					}

					tree := transaction.NewTree(s.storageBackend, ioRoot)
					defer tree.Close()

					txs, err = tree.GetTransactions(bctx)
					if err != nil {
						return err
					}

					tags, err = tree.GetTags(bctx)
					if err != nil {
						return err
					}

					return nil
				}, off)

				if err != nil {
					s.Logger.Error("can't get I/O root from storage",
						"err", err,
						"round", blk.Header.Round,
					)
					continue
				}
			}

			if err = s.backend.Index(s.ctx, blk.Header.Round, blk.Header.EncodedHash(), txs, tags); err != nil {
				s.Logger.Error("failed to index tags",
					"err", err,
					"round", blk.Header.Round,
				)
				continue
			}
		}
	}
}

func (s *Service) Start(storage storage.Backend) error {
	s.storageBackend = storage
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