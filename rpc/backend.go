package rpc

import (
	"context"

	"google.golang.org/grpc"

	"github.com/ethereum/go-ethereum/common"
	oasisCore "github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
)

// Backend implements the functionality shared within namespaces.
type Backend interface {
	GetBlockByNumber(ctx context.Context, blockNum uint64) (map[string]interface{}, error)
	GetBlockByHash(ctx context.Context, hash common.Hash) (map[string]interface{}, error)
}

type backend struct {
	sdk client.RuntimeClient
}

func NewBackend(conn *grpc.ClientConn) Backend {
	var runtimeID oasisCore.Namespace

	return &backend{
		sdk: client.New(conn, runtimeID),
	}
}

func (b *backend) GetBlockByNumber(ctx context.Context, blockNum uint64) (map[string]interface{}, error) {
	b.sdk.GetBlock(ctx, blockNum)
	return nil, nil
}

func (b *backend) GetBlockByHash(ctx context.Context, hash common.Hash) (map[string]interface{}, error) {
	return nil, nil
}
