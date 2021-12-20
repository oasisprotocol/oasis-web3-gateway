package utils

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestTopicsMatch(t *testing.T) {
	for _, tc := range []struct {
		topics      []common.Hash
		filter      [][]common.Hash
		shouldMatch bool
		msg         string
	}{
		{
			topics:      nil,
			filter:      nil,
			shouldMatch: true,
			msg:         "empty filter should match",
		},
		{
			topics:      nil,
			filter:      [][]common.Hash{{}},
			shouldMatch: false,
			msg:         "filter with one (wildcard) topic should not match",
		},
		{
			topics:      []common.Hash{common.HexToHash("0x123")},
			filter:      [][]common.Hash{{}},
			shouldMatch: true,
			msg:         "filter with one (wildcard) topic should match",
		},
		{
			topics:      []common.Hash{common.HexToHash("0x123")},
			filter:      [][]common.Hash{{common.HexToHash("0x123")}},
			shouldMatch: true,
			msg:         "filter with one topic should match",
		},
		{
			topics:      []common.Hash{common.HexToHash("0x123"), common.HexToHash("0x321")},
			filter:      [][]common.Hash{{common.HexToHash("0x123")}, {common.HexToHash("0x123")}},
			shouldMatch: false,
			msg:         "filter with 2nd topic mismatch should not match",
		},
		{
			topics:      []common.Hash{common.HexToHash("0x123"), common.HexToHash("0x321")},
			filter:      [][]common.Hash{{common.HexToHash("0x321")}, {common.HexToHash("0x123")}},
			shouldMatch: false,
			msg:         "filter with invalid order should not match",
		},
		{
			topics:      []common.Hash{common.HexToHash("0x123"), common.HexToHash("0x321")},
			filter:      [][]common.Hash{{common.HexToHash("0x123")}, {common.HexToHash("0x123"), common.HexToHash("0x321")}},
			shouldMatch: true,
			msg:         "filter with two topics should match",
		},
		{
			topics:      []common.Hash{common.HexToHash("0x123"), common.HexToHash("0x321")},
			filter:      [][]common.Hash{{common.HexToHash("0x123")}, {common.HexToHash("0x123"), common.HexToHash("0x321")}, {common.HexToHash("0x123")}},
			shouldMatch: false,
			msg:         "filter with three topics should not match",
		},
	} {
		require.Equal(t, tc.shouldMatch, TopicsMatch(&ethtypes.Log{Topics: tc.topics}, tc.filter), tc.msg)
	}
}
