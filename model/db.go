package model

// Block represents the relationship between block round and block hash.
type Block struct {
	Round uint64 `pg:",pk"`
	Hash  string
}

// Transaction represents the relationship between ethereum tx and oasis tx.
type Transaction struct {
	EthTx  string `pg:",pk"`
	Result *TxResult
}

// TxResult represents oasis tx result.
type TxResult struct {
	Hash  string
	Index uint32
	Round uint64
}
