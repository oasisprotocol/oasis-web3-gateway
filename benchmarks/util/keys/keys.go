package keys

import (
	"crypto/ecdsa"
	"crypto/sha512"
	"encoding/hex"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/sha3"

	memorySigner "github.com/oasisprotocol/oasis-core/go/common/crypto/signature/signers/memory"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature/ed25519"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature/secp256k1"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
)

const seedPrefix = "oasisprotocol/oasis-web3-gateway/benchmarks: "

// EVMBenchmarkKey is key generated for benchmarks.
type EVMBenchmarkKey struct {
	Signer  signature.Signer
	Address types.Address
	SigSpec types.SignatureAddressSpec

	EthAddress common.Address
	EthPrivate *ecdsa.PrivateKey
}

// ConsensusBenchmarkKey is key generated for benchmarks.
type ConsensusBenchmarkKey struct {
	Signer  signature.Signer
	Address types.Address
	SigSpec types.SignatureAddressSpec
}

// EVMBenchmarkKeys returns n benchmark keys.
func EVMBenchmarkKeys(n int) []*EVMBenchmarkKey {
	keys := make([]*EVMBenchmarkKey, 0, n)
	for i := 0; i < n; i++ {
		nk := newEVMBenchmarkKey(seedPrefix + fmt.Sprintf("%d", i))
		keys = append(keys, &nk)
	}

	return keys
}

// ConsensusBenchmarkKeys returns n consensus benchmark keys.
func ConsensusBenchmarkKeys(n int) []*ConsensusBenchmarkKey {
	keys := make([]*ConsensusBenchmarkKey, 0, n)
	for i := 0; i < n; i++ {
		nk := newConsensusBenchmarkKey(seedPrefix + fmt.Sprintf("cons-%d", i))
		keys = append(keys, &nk)
	}

	return keys
}

func newEVMBenchmarkKey(seed string) EVMBenchmarkKey {
	// Generate PK.
	pk := sha512.Sum512_256([]byte(seed))
	ethPk, _ := crypto.HexToECDSA(hex.EncodeToString(pk[:]))

	// Oasis signer and address.
	signer := secp256k1.NewSigner(pk[:])
	sigspec := types.NewSignatureAddressSpecSecp256k1Eth(signer.Public().(secp256k1.PublicKey))

	// Eth address.
	h := sha3.NewLegacyKeccak256()
	untaggedPk, _ := sigspec.Secp256k1Eth.MarshalBinaryUncompressedUntagged()
	h.Write(untaggedPk)
	var ethAddress [20]byte
	copy(ethAddress[:], h.Sum(nil)[32-20:])

	return EVMBenchmarkKey{
		Signer:     signer,
		Address:    types.NewAddress(sigspec),
		SigSpec:    sigspec,
		EthAddress: ethAddress,
		EthPrivate: ethPk,
	}
}

func newConsensusBenchmarkKey(seed string) ConsensusBenchmarkKey {
	signer := ed25519.WrapSigner(memorySigner.NewTestSigner(seed))
	sigspec := types.NewSignatureAddressSpecEd25519(signer.Public().(ed25519.PublicKey))

	return ConsensusBenchmarkKey{
		Signer:  signer,
		Address: types.NewAddress(sigspec),
		SigSpec: sigspec,
	}
}
