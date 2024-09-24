package metrics

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/filters"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/oasisprotocol/oasis-core/go/common/cbor"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/evm"

	"github.com/oasisprotocol/oasis-web3-gateway/indexer"
	"github.com/oasisprotocol/oasis-web3-gateway/rpc/eth"
	"github.com/oasisprotocol/oasis-web3-gateway/rpc/metrics"
	"github.com/oasisprotocol/oasis-web3-gateway/rpc/utils"
)

var requestHeights = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "oasis_web3_gateway_api_request_heights",
		Buckets: []float64{0, 1, 2, 3, 5, 10, 50, 100, 500, 1000},
		Help:    "Histogram of eth API request heights (difference from the latest  height).",
	},
	[]string{"method_name"},
)

type metricsWrapper struct {
	api eth.API

	logger  *logging.Logger
	backend indexer.Backend
}

var signedQueryCount = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "oasis_web3_gateway_signed_queries",
		Help: "Number of eth_call signed queries",
	},
)

func (m *metricsWrapper) latestAndRequestHeights(ctx context.Context, blockNum ethrpc.BlockNumber) (uint64, uint64, error) {
	var height uint64

	switch blockNum {
	case ethrpc.PendingBlockNumber, ethrpc.LatestBlockNumber:
		// Same as latest height by definition.
		return 0, 0, nil
	case ethrpc.EarliestBlockNumber:
		// Fetch earliest block.
		var err error
		height, err = m.backend.QueryLastRetainedRound(ctx)
		if err != nil {
			return 0, 0, err
		}
		// Continue below.
		fallthrough
	default:
		if int64(blockNum) < 0 {
			return 0, 0, eth.ErrMalformedBlockNumber
		}
		if height == 0 {
			height = uint64(blockNum)
		}

		// Fetch latest block height.
		// Note: as this is called async, the latest height could have changed since the request
		// was served, but this likely only results in a off-by-one measured height difference
		// which is good enough for instrumentation.
		latest, err := m.backend.QueryLastIndexedRound(ctx)
		if err != nil {
			return 0, 0, err
		}
		return latest, height, nil
	}
}

func (m *metricsWrapper) meassureRequestHeightDiff(method string, blockNum ethrpc.BlockNumber) {
	// Don't use the parent (request) context here, so that this method can run
	// without blocking the request.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	latest, request, err := m.latestAndRequestHeights(ctx, blockNum)
	if err != nil {
		m.logger.Debug("fetching latest and request heights error", "err", err)
		return
	}
	if request > latest {
		// This is a query for a non-existing block, skip.
		return
	}
	requestHeights.WithLabelValues(method).Observe(float64(latest - request))
}

// Accounts implements eth.API.
func (m *metricsWrapper) Accounts() (res []common.Address, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_accounts")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.Accounts()
	return
}

// BlockNumber implements eth.API.
func (m *metricsWrapper) BlockNumber(ctx context.Context) (res hexutil.Uint64, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_blockNumber")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.BlockNumber(ctx)
	return
}

// Check if the transaction (presumably via `eth_call` RPC) is a signed query.
func isSignedQuery(args utils.TransactionArgs) bool {
	var data *hexutil.Bytes
	switch {
	case args.Data != nil:
		data = args.Data
	case args.Input != nil:
		data = args.Input
	default:
		return false
	}
	if len(*data) > 1 && (*data)[0] == 0xa3 {
		var packed evm.SignedCallDataPack
		err := cbor.Unmarshal(*data, packed)
		if err == nil {
			return true
		}
	}
	return false
}

// Call implements eth.API.
func (m *metricsWrapper) Call(ctx context.Context, args utils.TransactionArgs, blockNrOrHash ethrpc.BlockNumberOrHash, so *utils.StateOverride) (res hexutil.Bytes, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_call")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	if isSignedQuery(args) {
		signedQueryCount.Inc()
	}

	res, err = m.api.Call(ctx, args, blockNrOrHash, so)
	return
}

// ChainId implements eth.API.
//
//nolint:revive,stylecheck
func (m *metricsWrapper) ChainId() (res *hexutil.Big, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_chainId")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.ChainId()
	return
}

// EstimateGas implements eth.API.
func (m *metricsWrapper) EstimateGas(ctx context.Context, args utils.TransactionArgs, blockNum *ethrpc.BlockNumber) (res hexutil.Uint64, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_estimateGas")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.EstimateGas(ctx, args, blockNum)
	return
}

// GasPrice implements eth.API.
func (m *metricsWrapper) GasPrice(ctx context.Context) (res *hexutil.Big, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_gasPrice")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GasPrice(ctx)
	return
}

// GetBalance implements eth.API.
func (m *metricsWrapper) GetBalance(ctx context.Context, address common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (res *hexutil.Big, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getBalance")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetBalance(ctx, address, blockNrOrHash)
	return
}

// GetBlockByHash implements eth.API.
func (m *metricsWrapper) GetBlockByHash(ctx context.Context, blockHash common.Hash, fullTx bool) (res map[string]interface{}, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getBlockByHash")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetBlockByHash(ctx, blockHash, fullTx)
	return
}

// GetBlockByNumber implements eth.API.
func (m *metricsWrapper) GetBlockByNumber(ctx context.Context, blockNum ethrpc.BlockNumber, fullTx bool) (res map[string]interface{}, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getBlockByNumber")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetBlockByNumber(ctx, blockNum, fullTx)

	// Measure request height difference from latest height.
	go m.meassureRequestHeightDiff("eth_getBlockByNumber", blockNum)

	return
}

// GetBlockHash implements eth.API.
func (m *metricsWrapper) GetBlockHash(ctx context.Context, blockNum ethrpc.BlockNumber, b bool) (res common.Hash, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getBlockHash")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetBlockHash(ctx, blockNum, b)

	// Measure request height difference from latest height.
	go m.meassureRequestHeightDiff("eth_getBlockHash", blockNum)

	return
}

// GetBlockTransactionCountByHash implements eth.API.
func (m *metricsWrapper) GetBlockTransactionCountByHash(ctx context.Context, blockHash common.Hash) (res hexutil.Uint, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getBlockTransationCountByHash")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetBlockTransactionCountByHash(ctx, blockHash)
	return
}

// GetBlockTransactionCountByNumber implements eth.API.
func (m *metricsWrapper) GetBlockTransactionCountByNumber(ctx context.Context, blockNum ethrpc.BlockNumber) (res hexutil.Uint, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getBlockTransationCountByNumber")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetBlockTransactionCountByNumber(ctx, blockNum)

	// Measure request height difference from latest height.
	go m.meassureRequestHeightDiff("eth_getBlockTransationCountByNumber", blockNum)

	return
}

// GetCode implements eth.API.
func (m *metricsWrapper) GetCode(ctx context.Context, address common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (res hexutil.Bytes, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getCode")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetCode(ctx, address, blockNrOrHash)
	return
}

// GetLogs implements eth.API.
func (m *metricsWrapper) GetLogs(ctx context.Context, filter filters.FilterCriteria) (res []*ethtypes.Log, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getLogs")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetLogs(ctx, filter)
	return
}

// GetStorageAt implements eth.API.
func (m *metricsWrapper) GetStorageAt(ctx context.Context, address common.Address, position hexutil.Big, blockNrOrHash ethrpc.BlockNumberOrHash) (res hexutil.Big, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getStorageAt")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetStorageAt(ctx, address, position, blockNrOrHash)
	return
}

// GetTransactionByBlockHashAndIndex implements eth.API.
func (m *metricsWrapper) GetTransactionByBlockHashAndIndex(ctx context.Context, blockHash common.Hash, index hexutil.Uint) (res *utils.RPCTransaction, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getTransactionByBlockHashAndIndex")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetTransactionByBlockHashAndIndex(ctx, blockHash, index)
	return
}

// GetTransactionByBlockNumberAndIndex implements eth.API.
func (m *metricsWrapper) GetTransactionByBlockNumberAndIndex(ctx context.Context, blockNum ethrpc.BlockNumber, index hexutil.Uint) (res *utils.RPCTransaction, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getTransactionByBlockNumberAndIndex")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetTransactionByBlockNumberAndIndex(ctx, blockNum, index)

	// Measure request height difference from latest height.
	go m.meassureRequestHeightDiff("eth_getTransactionByBlockNumberAndIndex", blockNum)

	return
}

// GetTransactionByHash implements eth.API.
func (m *metricsWrapper) GetTransactionByHash(ctx context.Context, hash common.Hash) (res *utils.RPCTransaction, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getTransactionByHash")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetTransactionByHash(ctx, hash)
	return
}

// GetTransactionCount implements eth.API.
func (m *metricsWrapper) GetTransactionCount(ctx context.Context, ethAddr common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (h *hexutil.Uint64, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getTransactionCount")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	h, err = m.api.GetTransactionCount(ctx, ethAddr, blockNrOrHash)
	return
}

// GetTransactionReceipt implements eth.API.
func (m *metricsWrapper) GetTransactionReceipt(ctx context.Context, txHash common.Hash) (res map[string]interface{}, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_getTransactionReceipt")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.GetTransactionReceipt(ctx, txHash)
	return
}

// Hashrate implements eth.API.
func (m *metricsWrapper) Hashrate() hexutil.Uint64 {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_hashrate")
	defer metrics.InstrumentCaller(r, s, f, i, d, nil)()

	return m.api.Hashrate()
}

// Mining implements eth.API.
func (m *metricsWrapper) Mining() bool {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_mining")
	defer metrics.InstrumentCaller(r, s, f, i, d, nil)()

	return m.api.Mining()
}

// SendRawTransaction implements eth.API.
func (m *metricsWrapper) SendRawTransaction(ctx context.Context, data hexutil.Bytes) (h common.Hash, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_sendRawTransaction")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	h, err = m.api.SendRawTransaction(ctx, data)
	return
}

// Syncing implements eth.API.
func (m *metricsWrapper) Syncing(ctx context.Context) (res interface{}, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_syncing")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.Syncing(ctx)
	return
}

// NewMetricsWrapper returns an instrumanted API service.
func NewMetricsWrapper(api eth.API, logger *logging.Logger, backend indexer.Backend) eth.API {
	return &metricsWrapper{
		api,
		logger,
		backend,
	}
}
