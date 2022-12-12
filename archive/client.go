// Package archive implements an archive node client.
package archive

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/oasisprotocol/oasis-web3-gateway/rpc/utils"
)

// Client is an archive node client backed by web, implementing a limited
// subset of rpc/eth.API, that is sufficient to support historical queries.
//
// All of the parameters that are `ethrpc.BlockNumberOrHash` just assume
// that the caller will handle converting to a block number, because they
// need to anyway, and historical estimate gas calls are not supported.
type Client struct {
	inner       *ethclient.Client
	latestBlock uint64
}

func (c *Client) LatestBlock() uint64 {
	return c.latestBlock
}

func (c *Client) GetStorageAt(
	ctx context.Context,
	address common.Address,
	position hexutil.Big,
	blockNr uint64,
) (hexutil.Big, error) {
	storageBytes, err := c.inner.StorageAt(
		ctx,
		address,
		common.BigToHash((*big.Int)(&position)),
		new(big.Int).SetUint64(blockNr),
	)
	if err != nil {
		return hexutil.Big{}, fmt.Errorf("archive: failed to query storage: %w", err)
	}

	// Oh for fuck's sake.
	var storageBig big.Int
	storageBig.SetBytes(storageBytes)
	return hexutil.Big(storageBig), nil
}

func (c *Client) GetBalance(
	ctx context.Context,
	address common.Address,
	blockNr uint64,
) (*hexutil.Big, error) {
	balance, err := c.inner.BalanceAt(
		ctx,
		address,
		new(big.Int).SetUint64(blockNr),
	)
	if err != nil {
		return nil, fmt.Errorf("archive: failed to query balance: %w", err)
	}

	return (*hexutil.Big)(balance), nil
}

func (c *Client) GetTransactionCount(
	ctx context.Context,
	address common.Address,
	blockNr uint64,
) (*hexutil.Uint64, error) {
	nonce, err := c.inner.NonceAt(
		ctx,
		address,
		new(big.Int).SetUint64(blockNr),
	)
	if err != nil {
		return nil, fmt.Errorf("archive: failed to query nonce: %w", err)
	}

	return (*hexutil.Uint64)(&nonce), nil
}

func (c *Client) GetCode(
	ctx context.Context,
	address common.Address,
	blockNr uint64,
) (hexutil.Bytes, error) {
	code, err := c.inner.CodeAt(
		ctx,
		address,
		new(big.Int).SetUint64(blockNr),
	)
	if err != nil {
		return nil, fmt.Errorf("archive: failed to query code: %w", err)
	}

	return hexutil.Bytes(code), nil
}

func (c *Client) Call(
	ctx context.Context,
	args utils.TransactionArgs,
	blockNr uint64,
) (hexutil.Bytes, error) {
	// You have got to be fucking shitting me, what in the actual fuck.

	if args.From == nil {
		return nil, fmt.Errorf("archive: no `from` in call")
	}
	callMsg := ethereum.CallMsg{
		From:      *args.From,
		To:        args.To,
		GasPrice:  (*big.Int)(args.GasPrice),
		GasFeeCap: (*big.Int)(args.MaxFeePerGas),
		GasTipCap: (*big.Int)(args.MaxPriorityFeePerGas),
		Value:     (*big.Int)(args.Value),
		// args.Nonce? I guess it can't be that important if there's no field for it.
	}
	if args.Gas != nil {
		callMsg.Gas = uint64(*args.Gas)
	}
	if args.Data != nil {
		callMsg.Data = []byte(*args.Data)
	}
	if args.Input != nil {
		// Data and Input are the same damn thing, Input is newer.
		callMsg.Data = []byte(*args.Input)
	}
	if args.AccessList != nil {
		callMsg.AccessList = *args.AccessList
	}

	result, err := c.inner.CallContract(
		ctx,
		callMsg,
		new(big.Int).SetUint64(blockNr),
	)
	if err != nil {
		return nil, fmt.Errorf("archive: failed to call contract: %w", err)
	}

	return hexutil.Bytes(result), nil
}

func (c *Client) Close() {
	c.inner.Close()
	c.inner = nil
}

func New(
	ctx context.Context,
	uri string,
	heightMax uint64,
) (*Client, error) {
	c, err := ethclient.DialContext(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("archive: failed to dial archival web3 node: %w", err)
	}

	var latestBlock uint64
	switch heightMax {
	case 0:
		if latestBlock, err = c.BlockNumber(ctx); err != nil {
			return nil, fmt.Errorf("archive: failed to query block number: %w", err)
		}
	default:
		latestBlock = heightMax
	}

	return &Client{
		inner:       c,
		latestBlock: latestBlock,
	}, nil
}
