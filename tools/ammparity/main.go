// Command ammparity prints what x/amm's keeper would pay out for a set of
// swaps, so that a client's reimplementation of the curve can be checked
// against the chain's arithmetic rather than against a reading of it.
//
// It exists for clients/markets, which quotes swaps in the browser. That
// console computes the output in JavaScript BigInt; the keeper computes it in
// cosmossdk.io/math. The two must agree exactly, and not approximately: a quote
// one base unit above what the keeper will pay is a min_amount_out the keeper
// rejects with ErrSlippage, after the trader has already signed. A quote one
// unit below is a trade that fills at worse than the price shown.
//
// The formula below is copied verbatim from x/amm/keeper/msg_server_swap.go,
// and copying it is the point: it is the same expression evaluated by the same
// arbitrary-precision integer type, so a difference in the output is a
// difference in the client rather than a difference in how this file was
// written. Its rounding direction is what protects the pool — see the keeper's
// own comment for why the algebraically identical subtraction form bleeds it.
//
//	go run ./tools/ammparity
//
// The rows it prints are pasted into clients/markets/markets.test.js as the
// KEEPER fixture. Re-run it if x/amm's swap arithmetic ever changes; the client
// test will then fail until the client is changed to match.
package main

import (
	"fmt"

	"cosmossdk.io/math"
)

// swapOut is x/amm/keeper/msg_server_swap.go's arithmetic, unaltered.
func swapOut(reserveIn, reserveOut, amountIn math.Int, swapFeeBps int64) math.Int {
	feeBps := math.NewInt(10000 - swapFeeBps)
	amountInAfterFee := amountIn.Mul(feeBps).Quo(math.NewInt(10000))
	return reserveOut.Mul(amountInAfterFee).Quo(reserveIn.Add(amountInAfterFee))
}

func i(s string) math.Int {
	v, ok := math.NewIntFromString(s)
	if !ok {
		panic("not an integer: " + s)
	}
	return v
}

func main() {
	// The first row is pool 1 on yamale-devnet-2. The rest are chosen to be
	// awkward: reserves whose product leaves a remainder, inputs of a few base
	// units where truncation is the whole answer, and a trade of half the
	// input reserve where the curve dominates.
	cases := [][3]string{
		{"20000000000", "30000000000", "1000000000"},
		{"1000000", "3000001", "7"},
		{"999999", "1000003", "13"},
		{"20000000000", "30000000000", "1"},
		{"20000000000", "30000000000", "10000000000"},
		{"10000000000", "15000000000", "123456789"},
		{"7", "11", "999999"},
		{"1000", "1001", "3"},
	}
	fees := []int64{0, 30, 500}

	for _, c := range cases {
		for _, f := range fees {
			fmt.Printf("  ['%s', '%s', '%s', %d, '%s'],\n",
				c[0], c[1], c[2], f, swapOut(i(c[0]), i(c[1]), i(c[2]), f))
		}
	}
}
