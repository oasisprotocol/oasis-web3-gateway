package model

// BlockRef represents the relationship between block round and block hash.
type BlockRef struct {
	Round uint64 `pg:",pk"`
	Hash  string
}

// TransactionRef represents the relationship between ethereum tx and oasis tx.
type TransactionRef struct {
	EthTxHash string `pg:",pk"`
	Index     uint32 `pg:",use_zero"`
	Round     uint64 `pg:",use_zero"`
}

// EthAccessTuple for ethereum AccessList.
type EthAccessTuple struct {
	Address     string
	StorageKeys []string
}

// EthTransaction is ethereum transaction.
type EthTransaction struct {
	Hash       string `pg:",pk"`
	Type       uint8  `pg:",use_zero"`
	ChainID    string
	Gas        uint64 `pg:",use_zero"`
	GasPrice   string
	GasTipCap  string
	GasFeeCap  string
	Nonce      uint64 `pg:",use_zero"`
	From       string
	To         string
	Value      string
	Data       string
	AccessList []EthAccessTuple
	V, R, S    string
}

// ContinuesIndexedRound Continues latest Indexed Block Round
type ContinuesIndexedRound struct {
	Round uint64 `pg:",pk"`
}
