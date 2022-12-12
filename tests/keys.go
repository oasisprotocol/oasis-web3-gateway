package tests

import (
	"crypto/ecdsa"
	"crypto/sha512"
	"encoding/hex"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature/secp256k1"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
	"golang.org/x/crypto/sha3"
)

// TestKey is a key used for testing.
type TestKey struct {
	Private      *ecdsa.PrivateKey
	EthAddress   common.Address
	OasisAddress types.Address
}

func newSecp256k1TestKey(seed string) TestKey {
	pk := sha512.Sum512_256([]byte(seed))
	ethPk, _ := crypto.HexToECDSA(hex.EncodeToString(pk[:]))

	// Oasis address.
	signer := secp256k1.NewSigner(pk[:])
	sigspec := types.NewSignatureAddressSpecSecp256k1Eth(signer.Public().(secp256k1.PublicKey))

	// Eth address.
	h := sha3.NewLegacyKeccak256()
	untaggedPk, _ := sigspec.Secp256k1Eth.MarshalBinaryUncompressedUntagged()
	h.Write(untaggedPk)
	var ethAddress [20]byte
	copy(ethAddress[:], h.Sum(nil)[32-20:])

	return TestKey{
		Private:      ethPk,
		OasisAddress: types.NewAddress(sigspec),
		EthAddress:   ethAddress,
	}
}

var (
	TestKey1 = newSecp256k1TestKey("oasis-web3-gateway/test-keys: 1")
	TestKey2 = newSecp256k1TestKey("oasis-web3-gateway/test-keys: 2")
	TestKey3 = newSecp256k1TestKey("oasis-web3-gateway/test-keys: 3")
)
