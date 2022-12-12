package web3

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/oasisprotocol/oasis-web3-gateway/version"
)

// API is the web3_ prefixed set of APIs in the Web3 JSON-RPC spec.
type API interface {
	// Sha3 returns the keccak-256 hash of the passed-in input.
	Sha3(input hexutil.Bytes) hexutil.Bytes
	// ClientVersion returns the current client info.
	ClientVersion() string
}

type publicAPI struct{}

// NewPublicAPI creates an instance of the Web3 API.
func NewPublicAPI() API {
	return &publicAPI{}
}

func (a *publicAPI) Sha3(input hexutil.Bytes) hexutil.Bytes {
	return crypto.Keccak256(input)
}

func (a *publicAPI) ClientVersion() string {
	return fmt.Sprintf("oasis/%s/%s", version.Software, version.Toolchain)
}
