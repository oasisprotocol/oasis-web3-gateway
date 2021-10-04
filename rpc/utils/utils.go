package utils

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
)

// ConstructBlock creates an ethereum block from a Oasis header and ethereum-formatted txs.
// TODO tx releated
func ConstructBlock(
	header block.Header,
) map[string]interface{} {

	bhash, _ := header.IORoot.MarshalBinary()
	bprehash, _ := header.PreviousHash.MarshalBinary()
	bshash, _ := header.StateRoot.MarshalBinary()
	btxhash, _ := header.MessagesHash.MarshalBinary()

	return map[string]interface{}{
		"number":     hexutil.Uint64(header.Round),
		"hash":       hexutil.Bytes(bhash),
		"parentHash": common.BytesToHash(bprehash),
		"nonce":      ethtypes.BlockNonce{},
		"sha3Uncles": ethtypes.EmptyUncleHash,
		//"logsBloom":        bloom,
		"stateRoot": hexutil.Bytes(bshash),
		//"miner":            validatorAddr,
		"mixHash":    common.Hash{},
		"difficulty": (*hexutil.Big)(big.NewInt(0)),
		"extraData":  "0x",
		//"size":             hexutil.Uint64(size),
		//"gasLimit":         hexutil.Uint64(gasLimit),
		//"gasUsed":          (*hexutil.Big)(gasUsed),
		"timestamp":        hexutil.Uint64(header.Timestamp),
		"transactionsRoot": hexutil.Bytes(btxhash),
		"receiptsRoot":     ethtypes.EmptyRootHash,

		"uncles": []common.Hash{},
		//"transactions":    transactions,
		"totalDifficulty": (*hexutil.Big)(big.NewInt(0)),
	}
}
