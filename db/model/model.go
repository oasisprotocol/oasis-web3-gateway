package model

// AccessTuple is the element of an AccessList.
type AccessTuple struct {
	Address     string
	StorageKeys []string
}

// Size returns the size of an AccessTuple in bytes.
func (at *AccessTuple) Size() int {
	sz := len(at.Address)
	for i := range at.StorageKeys {
		sz += len(at.StorageKeys[i])
	}
	return sz
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

// Size returns the size of an AccessList in bytes.
func (al AccessList) Size() int {
	var sz int
	for i := range al {
		sz += al[i].Size()
	}
	return sz
}

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

// Size returns the approximate size of a Transaction in bytes.
func (tx *Transaction) Size() int {
	sz := len(tx.Hash)
	sz++    // Type
	sz += 8 // Status
	sz += len(tx.ChainID)
	sz += len(tx.BlockHash)
	sz += 8 // Round
	sz += 4 // Index
	sz += 8 // Gas
	sz += len(tx.GasPrice)
	sz += len(tx.GasTipCap)
	sz += len(tx.GasFeeCap)
	sz += 8 // Nonce
	sz += len(tx.FromAddr)
	sz += len(tx.ToAddr)
	sz += len(tx.Value)
	sz += len(tx.Data)
	sz += tx.AccessList.Size()
	sz += len(tx.V)
	sz += len(tx.R)
	sz += len(tx.S)
	return sz
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
	Index     uint `bun:",pk"`
	Removed   bool
}

// Size returns the approximate size of a Log in bytes.
func (l *Log) Size() int {
	sz := len(l.Address)
	for i := range l.Topics {
		sz += len(l.Topics[i])
	}
	sz += len(l.Data)
	sz += 8 // Round
	sz += len(l.BlockHash)
	sz += len(l.TxHash)
	sz += 8 // TxIndex
	sz += 8 // Index
	sz++    // Removed
	return sz
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

// Size returns the approximate size of a Receipt in bytes.
func (r *Receipt) Size() int {
	sz := 8 // Status
	sz += 8 // CumulativeGasUsed
	sz += len(r.LogsBloom)
	for i := range r.Logs {
		sz += r.Logs[i].Size()
	}
	sz += len(r.TransactionHash)
	sz += len(r.BlockHash)
	sz += 8 // GasUsed
	sz += 8 // Type
	sz += 8 // Round (BlockNumber)
	sz += 8 // TransactionIndex (wtf, Transaction.Index is a uint32)
	sz += len(r.FromAddr)
	sz += len(r.ToAddr)
	sz += len(r.ContractAddress)
	return sz
}
