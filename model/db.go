package model

const (
	// Continues is the latest Indexed Block Round.
	Continues string = "continues"

	// LastRetained is the block with minimum height maintained.
	LastRetained string = "lastRetain"
)

type AccessList []AccessTuple

type ContinuesIndexedRound struct {
	Tip   string `bun:",pk"`
	Round uint64
}

// BlockRef represents the relationship between block round and block hash.
type BlockRef struct {
	Hash  string `bun:",pk"`
	Round uint64
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

type IndexedRoundWithTip struct {
	Tip   string `bun:",pk"`
	Round uint64
}

// Transaction is ethereum transaction in db.
type Transaction struct {
	Hash       string `bun:",pk"`
	Type       uint8  `bun:",use_zero"`
	Status     uint   `bun:",use_zero"` // tx/receipt status
	ChainID    string
	BlockHash  string
	Round      uint64 `bun:",use_zero"`
	Index      uint32 `bun:",use_zero"`
	Gas        uint64 `bun:",use_zero"`
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

// Block represents ethereum block in db.
type Block struct {
	Hash         string `bun:",pk"`
	Round        uint64 `bun:",use_zero"`
	Header       *Header
	Uncles       []*Header
	Transactions []*Transaction `bun:"rel:has-many"`
}

// Header represents ethereum block header in db.
type Header struct {
	ParentHash  string
	UncleHash   string
	Coinbase    string
	Root        string
	TxHash      string
	ReceiptHash string
	Bloom       string
	Difficulty  string
	Number      string
	GasLimit    uint64 `bun:",use_zero"`
	GasUsed     uint64 `bun:",use_zero"`
	Time        uint64 `bun:",use_zero"`
	Extra       string
	MixDigest   string
	Nonce       uint64 `bun:",use_zero"`
	BaseFee     string
}

// Log represents ethereum log in db.
type Log struct {
	Address   string
	Topics    []string
	Data      string
	Round     uint64 `bun:",use_zero"` // BlockNumber
	BlockHash string
	TxHash    string `bun:",pk"`
	TxIndex   uint   `bun:",use_zero"`
	Index     uint   `bun:",pk,use_zero"`
	Removed   bool
}

// Receipt represents ethereum receipt in db.
type Receipt struct {
	Status            uint   `bun:",use_zero"`
	CumulativeGasUsed uint64 `bun:",use_zero"`
	LogsBloom         string
	Logs              []*Log
	TransactionHash   string `bun:",pk"`
	BlockHash         string
	GasUsed           uint64 `bun:",use_zero"`
	Type              uint   `bun:",use_zero"`
	Round             uint64 `bun:",use_zero"` // BlockNumber
	TransactionIndex  uint64 `bun:",use_zero"`
	FromAddr          string
	ToAddr            string
	ContractAddress   string
}
