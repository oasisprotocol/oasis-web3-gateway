package eth

import (
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestRejectOverlong(t *testing.T) {
	var b hexutil.Big
	err := json.Unmarshal([]byte("\"0xf1111111122222222333333334444444455555555666666667777777788888888\""), &b)
	if err != nil {
		t.Logf("overlong unmarshal failed (expected): %v\n", err)
	} else {
		t.Errorf("overlong unmarshal didn't reject")
	}
}
