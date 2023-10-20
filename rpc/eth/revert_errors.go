package eth

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/evm"
)

const (
	revertErrorPrefix = "reverted: "
)

type RevertError struct {
	error
	Reason string `json:"reason"`
}

// ErrorData returns the ABI encoded error reason.
func (e *RevertError) ErrorData() interface{} {
	return e.Reason
}

func (api *publicAPI) newRevertErrorV1(revertErr error) *RevertError {
	// ABI encoded function.
	abiReason := []byte{0x08, 0xc3, 0x79, 0xa0} // Keccak256("Error(string)")

	// ABI encode the revert Reason string.
	revertReason := strings.TrimPrefix(revertErr.Error(), revertErrorPrefix)
	typ, _ := abi.NewType("string", "", nil)
	unpacked, err := (abi.Arguments{{Type: typ}}).Pack(revertReason)
	if err != nil {
		api.Logger.Error("failed to encode V1 revert error", "revert_reason", revertReason, "err", err)
		return &RevertError{
			error: revertErr,
		}
	}
	abiReason = append(abiReason, unpacked...)

	return &RevertError{
		error:  revertErr,
		Reason: hexutil.Encode(abiReason),
	}
}

func (api *publicAPI) newRevertErrorV2(revertErr error) *RevertError {
	b64Reason := strings.TrimPrefix(revertErr.Error(), revertErrorPrefix)
	reason, err := base64.RawStdEncoding.DecodeString(b64Reason)
	if err != nil {
		api.Logger.Error("failed to encode V2 revert error", "revert_reason", b64Reason, "err", err)
		return &RevertError{
			error: revertErr,
		}
	}

	return &RevertError{
		error:  revertErr,
		Reason: hexutil.Encode(reason),
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
		// XXX: This could be cached per epoch.
		var cerr error
		rtInfo, cerr = core.NewV1(api.client).RuntimeInfo(ctx)
		if cerr != nil {
			logger.Debug("failed querying runtime info", "err", err)
		}
	}

	// In EVM module version 1 the SDK decoded and returned readable revert reasons.
	// Since EVM version 2 the raw data is just base64 encoded and returned.
	// Changed in https://github.com/oasisprotocol/oasis-sdk/pull/1479
	var revertErr *RevertError
	switch {
	case rtInfo == nil || rtInfo.Modules[evm.ModuleName].Version > 1:
		// EVM version 2 onward (if no runtime info assume >1).
		revertErr = api.newRevertErrorV2(err)
	default:
		// EVM version 1 (deprecated).
		revertErr = api.newRevertErrorV1(err)
	}
	logger.Debug("failed to execute SimulateCall, reverted", "err", err, "reason", revertErr.Reason)
	return revertErr
}
