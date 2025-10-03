package eth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/evm"
)

const (
	revertErrorPrefix = "reverted: "
)

type revertError struct {
	error
	// Revert reason hex encoded.
	reason string
}

// ErrorData returns the hex encoded error reason.
func (e *revertError) ErrorData() interface{} {
	return e.reason
}

// ErrorCode is the error code for revert errors.
func (e *revertError) ErrorCode() int {
	return 3
}

func (api *publicAPI) newRevertErrorV1(revertErr error) *revertError {
	revertReason := strings.TrimPrefix(revertErr.Error(), revertErrorPrefix)

	var reason string
	var err error
	switch revertReason {
	case "":
		err = errors.New("execution reverted")
	default:
		err = fmt.Errorf("execution reverted: %v", revertReason)

		// ABI encode the revert Reason string.
		abiReason := []byte{0x08, 0xc3, 0x79, 0xa0} // Keccak256("Error(string)")
		typ, _ := abi.NewType("string", "", nil)
		packed, packErr := (abi.Arguments{{Type: typ}}).Pack(revertReason)
		if packErr != nil {
			api.Logger.Error("failed to encode V1 revert error", "revert_reason", revertReason, "err", packErr)
			err = revertErr
			break
		}
		abiReason = append(abiReason, packed...)
		reason = hexutil.Encode(abiReason)
	}

	return &revertError{
		error:  err,
		reason: reason,
	}
}

func (api *publicAPI) newRevertErrorV2(revertErr error) *revertError {
	// V2 format: 'reverted: <base64 encoded revert data>'.
	// Strip the 'reverted: ' prefix and b64 decode the reason.
	b64Reason := strings.TrimPrefix(revertErr.Error(), revertErrorPrefix)
	rawReason, err := base64.StdEncoding.DecodeString(b64Reason)
	if err != nil {
		api.Logger.Error("failed to decode V2 revert error", "revert_reason", b64Reason, "err", err)
		return &revertError{
			error: revertErr,
		}
	}

	// Try to parse the reason from the revert error as string.
	reason, unpackErr := abi.UnpackRevert(rawReason)
	switch unpackErr {
	case nil:
		// String reason.
		err = fmt.Errorf("execution reverted: %v", reason)
	default:
		// Custom solidity error, raw reason hex-encoded in the ErrorData.
		err = errors.New("execution reverted")
	}
	return &revertError{
		error:  err,
		reason: hexutil.Encode(rawReason),
	}
}

func (api *publicAPI) handleCallFailure(ctx context.Context, logger *logging.Logger, err error) error {
	if err == nil {
		return nil
	}

	// No special handling for non-reverts.
	if !strings.HasPrefix(err.Error(), revertErrorPrefix) {
		logger.Debug("failed to execute SimulateCall", "err", err)
		return err
	}

	// Query the indexer for runtime info, to obtain EVM module version.
	rtInfo := api.backend.RuntimeInfo()
	if rtInfo == nil {
		// In case the node has indexer disabled, query the runtime info on the fly.
		var cerr error
		rtInfo, cerr = api.source.CoreRuntimeInfo(ctx)
		if cerr != nil {
			logger.Debug("failed querying runtime info", "err", err)
		}
	}

	// In EVM module version 1 the SDK decoded and returned readable revert reasons.
	// Since EVM version 2 the raw data is just base64 encoded and returned.
	// Changed in https://github.com/oasisprotocol/oasis-sdk/pull/1479
	var revertErr *revertError
	switch {
	case rtInfo == nil || rtInfo.Modules[evm.ModuleName].Version > 1:
		// EVM version 2 onward (if no runtime info assume >1).
		revertErr = api.newRevertErrorV2(err)
	default:
		// EVM version 1 (deprecated).
		revertErr = api.newRevertErrorV1(err)
	}
	logger.Debug("failed to execute SimulateCall, reverted", "err", err, "reason", revertErr.reason)
	return revertErr
}
