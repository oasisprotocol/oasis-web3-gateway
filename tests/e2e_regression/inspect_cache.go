package main

import (
	"fmt"
	"os"

	"github.com/cockroachdb/pebble"

	"github.com/oasisprotocol/oasis-core/go/common/cbor"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run inspect_cache.go <path-to-pebble-cache>")
		os.Exit(1)
	}

	db, err := pebble.Open(os.Args[1], &pebble.Options{})
	if err != nil {
		fmt.Printf("Error opening pebble db: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	iter, err := db.NewIter(nil)
	if err != nil {
		fmt.Printf("Error creating iterator: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = iter.Close() }()

	minGasPriceCount := 0
	otherCount := 0
	keys := make(map[string]int)
	minGasPriceRounds := make(map[uint64]bool)

	for iter.First(); iter.Valid(); iter.Next() {
		key := iter.Key()
		var decoded []interface{}
		if err := cbor.Unmarshal(key, &decoded); err == nil {
			if len(decoded) > 0 {
				method := fmt.Sprintf("%v", decoded[0])
				keys[method]++
				if method == "CoreMinGasPrice" {
					minGasPriceCount++
					if len(decoded) > 1 {
						if params, ok := decoded[1].([]interface{}); ok && len(params) > 0 {
							if round, ok := params[0].(uint64); ok {
								minGasPriceRounds[round] = true
							}
						}
					}
					if minGasPriceCount <= 5 {
						fmt.Printf("Found CoreMinGasPrice key: %v\n", decoded)
					}
				} else {
					otherCount++
				}
			}
		}
	}

	// Check for missing rounds in the last 10 blocks
	fmt.Printf("\nChecking last 10 block rounds for CoreMinGasPrice:\n")
	maxRound := uint64(0)
	for round := range minGasPriceRounds {
		if round > maxRound {
			maxRound = round
		}
	}
	for i := uint64(0); i < 10; i++ {
		round := maxRound - i
		if minGasPriceRounds[round] {
			fmt.Printf("  Round %d: CACHED\n", round)
		} else {
			fmt.Printf("  Round %d: MISSING\n", round)
		}
	}

	fmt.Printf("\nSummary of cached methods:\n")
	for method, count := range keys {
		fmt.Printf("  %s: %d\n", method, count)
	}
	fmt.Printf("\nTotal CoreMinGasPrice entries: %d\n", minGasPriceCount)
}
