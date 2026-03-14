package universe_test

import (
	"fmt"

	"github.com/penny-vault/pvbt/universe"
)

func ExampleNewStatic() {
	// Create a universe from a fixed set of tickers.
	u := universe.NewStatic("SPY", "TLT", "GLD")
	fmt.Println(u)
}

func ExampleNewStatic_withNamespace() {
	// Tickers can include a namespace prefix to specify the data source.
	u := universe.NewStatic("FRED:DGS3MO", "FRED:DGS10")
	fmt.Println(u)
}
