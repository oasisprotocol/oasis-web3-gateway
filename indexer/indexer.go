package indexer

import (
	"context"
	"errors"
	"time"

	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/service"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage"
)

const (
	storageRequestTimeout    = 5 * time.Second
	storageRetryTimeout      = 1 * time.Second
	CheckPruningTimeInterval = 60 * time.Second
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
	runtimeID     common.Namespace
	backend       Backend
	client        client.RuntimeClient
	ctx           context.Context
	cancelCtx     context.CancelFunc
	enablePruning bool
	pruningStep   uint64
}

func (s *Service) indexBlock(round uint64) error {
	blk, err1 := s.client.GetBlock(s.ctx, round)
	if err1 != nil {
		return ErrGetBlockFailed
	}

	txs, err2 := s.client.GetTransactionsWithResults(s.ctx, blk.Header.Round)
	if err2 != nil {
		return ErrGetTransactionsFailed
	}

	err3 := s.backend.Index(blk, txs)

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

func (s *Service) puring() {
	s.Logger.Debug("start purning!")
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(CheckPruningTimeInterval):
			indexed := s.backend.QueryIndexedRound()
			if indexed > s.pruningStep {
				pruningCheckPoint := indexed - s.pruningStep
				s.backend.Pruning(pruningCheckPoint)
			}
		}
	}
}

func (s *Service) periodIndexWorker() {
ContinusFor:
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(storageRequestTimeout):
			latest, err := s.getRoundLatest()
			if err != nil {
				s.Logger.Info("Can't get round latest, continue!")
				continue
			}

			indexed := s.backend.QueryIndexedRound()
			if latest < indexed {
				panic("This is a new chain, please clear the db first!")
			}
			if latest == indexed {
				continue
			}
			continueIndex := make(chan int, 1)
			continueIndex <- 1
			s.Logger.Info("Start index!")

		IndexFor:
			for {
				select {
				case <-s.ctx.Done():
					return
				case <-continueIndex:
					indexed++
					err := s.indexBlock(indexed)
					if err != nil {
						indexed--
						s.Logger.Info("IndexedBlock failed, continue!")
						goto IndexFor
					}
					if latest == indexed {
						goto ContinusFor
					}
					continueIndex <- 1
				case <-time.After(storageRetryTimeout):
					indexed++
					err := s.indexBlock(indexed)
					if err != nil {
						indexed--
						s.Logger.Info("IndexedBlock failed, continue!")
						goto IndexFor
					}
					if latest == indexed {
						goto ContinusFor
					}
					continueIndex <- 1
				}
			}
		}
	}
}

func (s *Service) Start() {
	go s.periodIndexWorker()

	if s.enablePruning {
		go s.puring()
	}
}

func (s *Service) Stop() {
	s.cancelCtx()
}

// New creates a new indexer service.
func New(backendFactory BackendFactory,
	client client.RuntimeClient,
	runtimeID common.Namespace,
	storage storage.Storage,
	enablePruning bool,
	pruningStep uint64) (*Service, Backend, error) {

	backend, err := backendFactory(runtimeID, storage)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancelCtx := context.WithCancel(context.Background())

	s := &Service{
		BaseBackgroundService: *service.NewBaseBackgroundService("gateway/indexer"),
		runtimeID:             runtimeID,
		backend:               backend,
		client:                client,
		ctx:                   ctx,
		cancelCtx:             cancelCtx,
		enablePruning:         enablePruning,
		pruningStep:           pruningStep,
	}
	s.Logger = s.Logger.With("runtime_id", s.runtimeID.String())

	return s, backend, nil
}
