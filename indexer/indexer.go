package indexer

import (
	"context"
	"errors"
	"time"

	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/service"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
)

const (
	storageRequestTimeout = 5 * time.Second
	storageRetryTimeout   = 120 * time.Second
)

const RoundLatest = client.RoundLatest

var (
	ErrGetBlockFailed        = errors.New("GetBlock failed")
	ErrGetTransactionsFailed = errors.New("GetTransactions failed")
	ErrIndexedFailed         = errors.New("Index block failed")
)

// Service is an indexer service.
type Service struct {
	service.BaseBackgroundService
	runtimeID common.Namespace
	backend   Backend
	client    client.RuntimeClient
	ctx       context.Context
	cancelCtx context.CancelFunc
	stopFlag  bool
}

func (s *Service) indexBlock(round uint64) error {
	blk, err1 := s.client.GetBlock(s.ctx, round)
	if err1 != nil {
		return ErrGetBlockFailed
	}

	txs, err2 := s.client.GetTransactions(s.ctx, blk.Header.Round)
	if err2 != nil {
		return ErrGetTransactionsFailed
	}

	err3 := s.backend.Index(blk.Header.Round, blk.Header.EncodedHash(), txs)
	if err3 != nil {
		return ErrIndexedFailed
	}

	return nil
}

func (s *Service) getRoundLatest() (uint64, error) {
	blk, err := s.client.GetBlock(s.ctx, RoundLatest)
	if err != nil {
		return 0, err
	}

	return blk.Header.Round, nil
}

func (s *Service) periodIndexWorker() {
	for {
		if s.stopFlag {
			break
		}

		latest, err := s.getRoundLatest()
		if err != nil {
			time.Sleep(storageRequestTimeout)
			continue
		}

		indexed := s.backend.QueryIndexedRound()
		if latest == indexed {
			time.Sleep(storageRetryTimeout)
			continue
		}

		for {
			if s.stopFlag || latest == indexed {
				break
			}

			indexed++
			err := s.indexBlock(indexed)
			if err != nil {
				indexed--
				time.Sleep(storageRequestTimeout)
				continue
			}
		}
	}
}

func (s *Service) Start() {
	go s.periodIndexWorker()
}

func (s *Service) Stop() {
	s.cancelCtx()
	s.stopFlag = true
}

// New creates a new indexer service.
func New(dataDir string, runtimeID common.Namespace, backendFactory BackendFactory, client client.RuntimeClient) (*Service, error) {
	backend, err := backendFactory(dataDir, runtimeID, nil)
	if err != nil {
		return nil, err
	}

	ctx, cancelCtx := context.WithCancel(context.Background())

	s := &Service{
		BaseBackgroundService: *service.NewBaseBackgroundService("gateway/indexer"),
		runtimeID:             runtimeID,
		backend:               backend,
		client:                client,
		ctx:                   ctx,
		cancelCtx:             cancelCtx,
		stopFlag:              false,
	}
	s.Logger = s.Logger.With("runtime_id", s.runtimeID.String())

	return s, nil
}
