package model

// BlockRef represents the relationship between block round and block hash.
type BlockRef struct {
	Round uint64
	Hash  string
}

// TransactionRef represents the relationship between ethereum tx and oasis tx.
type TransactionRef struct {
	EthTxHash string
	Result    TxResult
}

// TxResult represents oasis tx result.
type TxResult struct {
	Hash  string
	Index uint64
	Round uint64
}
