package web3

import (
	"fmt"
	"runtime"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/starfishlabs/oasis-evm-web3-gateway/version"
)

// PublicAPI is the web3_ prefixed set of APIs in the Web3 JSON-RPC spec.
type PublicAPI struct{}

// NewPublicAPI creates an instance of the Web3 API.
func NewPublicAPI() *PublicAPI {
	return &PublicAPI{}
}

// Sha3 returns the keccak-256 hash of the passed-in input.
func (a *PublicAPI) Sha3(input hexutil.Bytes) hexutil.Bytes {
	return crypto.Keccak256(input)
}

// ClientVersion returns the current client info.
func (a *PublicAPI) ClientVersion() string {
	return fmt.Sprintf("oasis/%s/%s", version.Version(), runtime.Version())
}
