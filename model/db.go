package model

// TODO: block and Transaction complete  definition

// Block is a temp block definition
type Block struct {
	Hash    string
	Round   uint64
	RawData []byte
}

// Transaction is a temp transaction definition
type Transaction struct {
	Hash    string
	Block   string
	Round   uint64
	Index   uint32
	RawData []byte
}
