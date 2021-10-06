package model

// Block represents the relationship between block round and block hash.
type Block struct {
	Round uint64 `pg:",pk"`
	Hash  string
}

type EthAccessTuple struct {
	Address     string
	StorageKeys []string
}

type EthTx struct {
	Hash       string `pg:",pk"`
	Type       uint8
	ChainID    string
	Gas        uint64
	GasPrice   string
	GasTipCap  string
	GasFeeCap  string
	Nonce      uint64
	To         string
	Value      string
	Data       string
	AccessList []EthAccessTuple
	V, R, S    string
}

// Transaction represents the relationship between ethereum tx and oasis tx.
type Transaction struct {
	EthTxHash string `pg:",pk"`
	Result    *TxResult
}

// TxResult represents oasis tx result.
type TxResult struct {
	Hash  string
	Index uint32
	Round uint64
}
