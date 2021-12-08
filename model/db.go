package model

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

// Continues is the latest Indexed Block Round.
const Continues string = "continues"

// LastRetained is the block with minimum height maintained.
const LastRetained string = "lastRetain"

type IndexedRoundWithTip struct {
	Tip   string `bun:",pk"`
	Round uint64
}

type AccessList []AccessTuple

// Transaction is ethereum transaction.
type Transaction struct {
	Hash       string `bun:",pk"`
	Type       uint8
	Status     uint // tx/receipt status
	ChainID    string
	BlockHash  string
	Round      uint64
	Index      uint32
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

// Block represents ethereum block.
type Block struct {
	Hash         string `bun:",pk"`
	Round        uint64
	Header       *Header
	Transactions []*Transaction `bun:"rel:has-many,join:hash=block_hash"`
}

// Header represents ethereum block header.
type Header struct {
	ParentHash  string
	UncleHash   string
	Coinbase    string
	Root        string
	TxHash      string
	ReceiptHash string
	Bloom       string
	Difficulty  string
	Number      uint64
	GasLimit    uint64
	GasUsed     uint64
	Time        uint64
	Extra       string
	MixDigest   string
	Nonce       uint64
	BaseFee     string
}

// Log represents ethereum log in db.
type Log struct {
	Address   string
	Topics    []string
	Data      string
	Round     uint64 // BlockNumber
	BlockHash string
	TxHash    string `bun:",pk"`
	TxIndex   uint
	Index     uint `bun:",pk,allowzero"`
	Removed   bool
}

type Receipt struct {
	Status            uint
	CumulativeGasUsed uint64
	LogsBloom         string
	Logs              []*Log `bun:"rel:has-many,join:transaction_hash=tx_hash"`
	TransactionHash   string `bun:",pk"`
	BlockHash         string
	GasUsed           uint64
	Type              uint
	Round             uint64 // BlockNumber
	TransactionIndex  uint64
	FromAddr          string
	ToAddr            string
	ContractAddress   string
}
