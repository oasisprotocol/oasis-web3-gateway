package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	ethAccounts "github.com/ethereum/go-ethereum/accounts"
	hdwallet "github.com/miguelmota/go-ethereum-hdwallet"
	"github.com/tyler-smith/go-bip39"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/cbor"
	coreSignature "github.com/oasisprotocol/oasis-core/go/common/crypto/signature"
	cmnGrpc "github.com/oasisprotocol/oasis-core/go/common/grpc"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	consensus "github.com/oasisprotocol/oasis-core/go/consensus/api"
	"github.com/oasisprotocol/oasis-core/go/consensus/api/transaction"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature/ed25519"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature/secp256k1"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature/sr25519"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	consAccClient "github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/consensusaccounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/testing"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
)

const (
	highGasAmount  = 1000000
	derivationPath = "m/44'/60'/0'/0/%d"
)

var (
	// Number of keys to derive from the mnemonic, if provided.
	numMnemonicDerivations = 5

	// Origin account for depositing tokens from.
	srcAccount = testing.Alice
)

func sigspecForSigner(signer signature.Signer) types.SignatureAddressSpec {
	switch pk := signer.Public().(type) {
	case ed25519.PublicKey:
		return types.NewSignatureAddressSpecEd25519(pk)
	case secp256k1.PublicKey:
		return types.NewSignatureAddressSpecSecp256k1Eth(pk)
	case sr25519.PublicKey:
		return types.NewSignatureAddressSpecSr25519(pk)
	default:
		panic(fmt.Sprintf("unsupported signer type: %T", pk))
	}
}

// GetChainContext returns the runtime chain context.
func GetChainContext(ctx context.Context, rtc client.RuntimeClient) (signature.Context, error) {
	info, err := rtc.GetInfo(ctx)
	if err != nil {
		return signature.RawContext{}, err
	}
	return info.ChainContext, nil
}

// EstimateGas estimates the amount of gas the transaction will use.
// Returns modified transaction that has just the right amount of gas.
func EstimateGas(ctx context.Context, rtc client.RuntimeClient, tx types.Transaction, extraGas uint64) types.Transaction {
	var gas uint64
	oldGas := tx.AuthInfo.Fee.Gas
	// Set the starting gas to something high, so we don't run out.
	tx.AuthInfo.Fee.Gas = highGasAmount
	// Estimate gas usage.
	gas, err := core.NewV1(rtc).EstimateGas(ctx, client.RoundLatest, &tx, true)
	if err != nil {
		tx.AuthInfo.Fee.Gas = oldGas + extraGas
		return tx
	}
	// Specify only as much gas as was estimated.
	tx.AuthInfo.Fee.Gas = gas + extraGas
	return tx
}

// SignAndSubmitConsensusTx signs and submits the given consensus transaction.
func SignAndSubmitConsensusTx(ctx context.Context, cs consensus.ClientBackend, signer coreSignature.Signer, tx *transaction.Transaction) error {
	chainCtx, err := cs.GetChainContext(ctx)
	if err != nil {
		return err
	}

	sigCtx := coreSignature.Context([]byte(fmt.Sprintf("%s for chain %s", transaction.SignatureContext, chainCtx)))
	signed, err := coreSignature.SignSigned(signer, sigCtx, tx)
	if err != nil {
		return err
	}

	return cs.SubmitTx(ctx, &transaction.SignedTransaction{Signed: *signed})
}

// SignAndSubmitTx signs and submits the given transaction.
// Gas estimation is done automatically.
func SignAndSubmitTx(ctx context.Context, rtc client.RuntimeClient, signer signature.Signer, tx types.Transaction, extraGas uint64) (cbor.RawMessage, error) {
	// Get chain context.
	chainCtx, err := GetChainContext(ctx, rtc)
	if err != nil {
		return nil, err
	}

	// Get current nonce for the signer's account.
	ac := accounts.NewV1(rtc)
	nonce, err := ac.Nonce(ctx, client.RoundLatest, types.NewAddress(sigspecForSigner(signer)))
	if err != nil {
		return nil, err
	}
	tx.AppendAuthSignature(sigspecForSigner(signer), nonce)

	// Estimate gas.
	etx := EstimateGas(ctx, rtc, tx, extraGas)

	// Sign the transaction.
	stx := etx.PrepareForSigning()
	if err = stx.AppendSign(chainCtx, signer); err != nil {
		return nil, err
	}

	// Submit the signed transaction.
	var result cbor.RawMessage
	if result, err = rtc.SubmitTx(ctx, stx.UnverifiedTransaction()); err != nil {
		return nil, err
	}
	return result, nil
}

// printSummary prints deposit summary similar to ganache-cli.
func printSummary(addresses []string, keys []string, baseAmount types.BaseUnits, mnemonic string) {
	fmt.Println("Available Accounts")
	fmt.Println("==================")

	amount := new(big.Int).Div(baseAmount.Amount.ToBigInt(), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))

	for i, a := range addresses {
		fmt.Printf("(%d) %s (%d TEST)\n", i, a, amount)
	}

	if len(keys) != 0 {
		fmt.Println("\nPrivate Keys")
		fmt.Println("==================")

		for i, k := range keys {
			fmt.Printf("(%d) 0x%s\n", i, k)
		}

		fmt.Println("\nHD Wallet")
		fmt.Println("==================")
		fmt.Printf("Mnemonic:\t%s\n", mnemonic)
		fmt.Printf("Base HD Path:\t%s\n", derivationPath)
	}
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: oasis-deposit [ options ... ]")
		fmt.Fprintln(os.Stderr, "Deposit native tokens from Cory testing account on the consensus layer to the given address on ParaTime.")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	amount := flag.String("amount", "10_000_000_000_000_000_000_000", "amount to deposit in ParaTime base units")
	sock := flag.String("sock", "", "oasis-node internal UNIX socket address")
	rtid := flag.String("rtid", "8000000000000000000000000000000000000000000000000000000000000000", "Runtime ID")
	to := flag.String("to", "", "comma-separated deposit addresses in 0x or oasis1 format or mnemonic phrase. If none provided, new mnemonic will be generated")
	useTestMnemonic := flag.Bool("test-mnemonic", true, "Use the standard test mnemonic (test test test test test test test test test test test junk)")
	flag.IntVar(&numMnemonicDerivations, "n", numMnemonicDerivations, "number of addresses to derive from mnemonic")
	flag.Parse()

	*amount = strings.ReplaceAll(*amount, "_", "")
	if (*amount == "") || (*sock == "") {
		flag.PrintDefaults()
		os.Exit(1)
	}
	qnt := *quantity.NewQuantity()
	amountBigInt, succ := new(big.Int).SetString(*amount, 0)
	if !succ {
		panic(fmt.Sprintf("can't parse amount %s, obtained value %s", *amount, amountBigInt.String()))
	}
	if err := qnt.FromBigInt(amountBigInt); err != nil {
		panic(fmt.Sprintf("can't parse quantity: %s", err))
	}
	baseAmount := types.NewBaseUnits(qnt, types.NativeDenomination)

	var runtimeID common.Namespace
	if err := runtimeID.UnmarshalHex(*rtid); err != nil {
		panic(fmt.Sprintf("can't decode runtime ID: %s", err))
	}

	conn, err := cmnGrpc.Dial(*sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(fmt.Sprintf("can't connect to socket: %s", err))
	}
	defer conn.Close()

	rtc := client.New(conn, runtimeID)
	consAcc := consAccClient.NewV1(rtc)

	ctx, cancelFn := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancelFn()

	toAddresses := []string{*to}
	toPrivateKeys := []string{}
	if *useTestMnemonic {
		*to = "test test test test test test test test test test test junk"
	}
	if *to == "" {
		// Address or mnemonic not provided. Generate a short mnemonic.
		entropy, _ := bip39.NewEntropy(128)
		mnemonic, _ := bip39.NewMnemonic(entropy)
		*to = mnemonic
	}
	if strings.Contains(*to, ",") {
		toAddresses = strings.Split(*to, ",")
	} else if strings.Contains(*to, " ") {
		// Check, if mnemonic was provided instead of the address.
		var wallet *hdwallet.Wallet
		wallet, err = hdwallet.NewFromMnemonic(*to)
		if err != nil {
			panic(fmt.Sprintf("failed to create hdwallet: %s", err))
		}

		toAddresses = []string{}
		for i := 0; i < numMnemonicDerivations; i++ {
			path := hdwallet.MustParseDerivationPath(fmt.Sprintf(derivationPath, i))
			var account ethAccounts.Account
			account, err = wallet.Derive(path, false)
			if err != nil {
				panic(fmt.Sprintf("failed to derive key from mnemonic: %s", err))
			}
			toAddresses = append(toAddresses, account.Address.Hex())

			var privateKey string
			privateKey, err = wallet.PrivateKeyHex(account)
			if err != nil {
				panic(fmt.Sprintf("failed to obtain private key for account %s: %s", account.Address, err))
			}
			toPrivateKeys = append(toPrivateKeys, privateKey)
		}
	}

	for _, a := range toAddresses {
		var addr types.Address
		if !strings.HasPrefix(a, "oasis1") {
			// Ethereum address provided.
			a = strings.TrimPrefix(a, "0x")
			var aBytes []byte
			aBytes, err = hex.DecodeString(a)
			if err != nil {
				panic(fmt.Sprintf("unmarshal hex err: %s", err))
			}
			addr = types.NewAddressRaw(types.AddressV0Secp256k1EthContext, aBytes)
		} else if err = addr.UnmarshalText([]byte(a)); err != nil {
			panic(fmt.Sprintf("unmarshal addr err: %s", err))
		}
		txb := consAcc.Deposit(&addr, baseAmount).SetFeeConsensusMessages(1)
		_, err = SignAndSubmitTx(ctx, rtc, srcAccount.Signer, *txb.GetTransaction(), 0)
		if err != nil {
			panic(fmt.Sprintf("can't deposit: %s", err))
		}
	}

	printSummary(toAddresses, toPrivateKeys, baseAmount, *to)
}
