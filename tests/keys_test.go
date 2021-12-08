package tests

import (
	"fmt"
	"testing"
)

func TestPrintTestAddresses(t *testing.T) {
	fmt.Println("Test Key 1:")
	fmt.Printf("Eth: %v Oasis: %v\n", TestKey1.EthAddress, TestKey1.OasisAddress)
	fmt.Println("Test Key 2:")
	fmt.Printf("Eth: %v Oasis: %v\n", TestKey2.EthAddress, TestKey2.OasisAddress)
	fmt.Println("Test Key 3:")
	fmt.Printf("Eth: %v Oasis: %v\n", TestKey3.EthAddress, TestKey3.OasisAddress)
}
