package rpc

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/oasisprotocol/oasis-web3-gateway/tests"
)

func TestEth_SubscribeNewHead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), OasisBlockTimeout)
	defer cancel()

	client := localClient(t, true)

	heads := make(chan *types.Header)
	sub, err := client.SubscribeNewHead(ctx, heads)
	require.NoError(t, err, "subscribe new head")
	defer sub.Unsubscribe()

	select {
	case head := <-heads:
		t.Logf("recv new head: %d", head.Number.Int64())
		require.Greater(t, head.Number.Int64(), int64(0), "new head")
	case <-sub.Err():
		t.Fatal("newhead connection reset")
	case <-time.After(OasisBlockTimeout):
		t.Fatal("no new head received")
	}
}

func TestEth_SubscribeLogs(t *testing.T) {
	if tests.TestsConfig.Gateway.ExposeOasisRPCs {
		t.Skip("contract tests w/ c10lity require compat lib to be integrated")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), OasisBlockTimeout)
	defer cancel()

	code := common.FromHex(strings.TrimSpace(evmEventsTestCompiledHex))
	client := localClient(t, true)
	round, err := client.BlockNumber(ctx)
	require.NoError(t, err)

	// The filter should receive contract events as
	// `event Log(address indexed sender, string message);`
	logs := make(chan types.Log)
	filterQuery := ethereum.FilterQuery{
		Addresses: nil,
		FromBlock: new(big.Int).SetUint64(round),
		ToBlock:   nil,
		Topics:    [][]common.Hash{nil, {common.BytesToHash(tests.TestKey1.EthAddress[:])}},
	}
	sub, err := client.SubscribeFilterLogs(ctx, filterQuery, logs)
	require.NoError(t, err, "subscribe new head")
	defer sub.Unsubscribe()

	go func() {
		chainID, err := client.ChainID(context.Background())
		require.Nil(t, err, "get chainid")
		nonce, err := client.NonceAt(context.Background(), tests.TestKey1.EthAddress, nil)
		require.Nil(t, err, "get nonce failed")

		// Contract create transaction
		tx := types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			Value:    big.NewInt(0),
			Gas:      1_000_000,
			GasPrice: GasPrice,
			Data:     code,
		})
		signer := types.LatestSignerForChainID(chainID)
		signature, err := crypto.Sign(signer.Hash(tx).Bytes(), tests.TestKey1.Private)
		require.Nil(t, err, "sign tx")

		signedTx, err := tx.WithSignature(signer, signature)
		require.Nil(t, err, "pack tx")

		err = client.SendTransaction(context.Background(), signedTx)
		require.NoError(t, err, "send transaction failed")
	}()

	select {
	case log := <-logs:
		t.Logf("recv log %s", log.Data)
		require.NotEmpty(t, log.Data)
		require.Equal(t, log.Topics[1], common.BytesToHash(tests.TestKey1.EthAddress[:]))
	case <-sub.Err():
		t.Fatal("logs connection reset")
	case <-time.After(OasisBlockTimeout):
		t.Fatal("recv logs timeout")
	}
}
