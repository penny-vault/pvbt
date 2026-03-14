package tradecron_test

import (
	"fmt"

	"github.com/penny-vault/pvbt/tradecron"
)

func ExampleNew_monthEnd() {
	tc, err := tradecron.New("@monthend", tradecron.RegularHours)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(tc)
}

func ExampleNew_dailyClose() {
	tc, err := tradecron.New("@close * * *", tradecron.RegularHours)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(tc)
}

func ExampleNew_weeklyRebalance() {
	// First trading day of each week at market open
	tc, err := tradecron.New("@weekbegin", tradecron.RegularHours)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(tc)
}

func ExampleNew_intraday() {
	// Every 5 minutes during regular trading hours
	tc, err := tradecron.New("*/5 * * * *", tradecron.RegularHours)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(tc)
}
