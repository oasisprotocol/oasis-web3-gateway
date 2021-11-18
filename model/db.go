package model

type AccessList []AccessTuple

// Continues is the latest Indexed Block Round.
const Continues string = "tip"

type ContinuesIndexedRound struct {
	Tip   string `bun:",pk"`
	Round uint64
}

// BlockRef represents the relationship between block round and block hash.
type BlockRef struct {
	Round uint64 `bun:",pk"`
	Hash  string
}

// TransactionRef represents the relationship between ethereum tx and oasis tx.
type TransactionRef struct {
	EthTxHash string `bun:",pk"`
	Index     uint32
	Round     uint64
	BlockHash string
}

// AccessTuple is the element of an AccessList.
type AccessTuple struct {
	Address     string
	StorageKeys []string
}

// Transaction is ethereum transaction.
type Transaction struct {
	Hash       string `bun:",pk"`
	Type       uint8
	ChainID    string
	Gas        uint64
	GasPrice   string
	GasTipCap  string
	GasFeeCap  string
	Nonce      uint64
	FromAddr   string
	ToAddr     string
	Value      string
	Data       string
	AccessList AccessList
	V, R, S    string
}
