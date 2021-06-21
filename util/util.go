package util

import (
	"strings"
	"time"
)

// Struct used for sorting tickers by a float64 value (i.e. momentum)
type Pair struct {
	Key   string
	Value float64
}

type PairList []Pair

func (p PairList) Len() int           { return len(p) }
func (p PairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p PairList) Less(i, j int) bool { return p[i].Value < p[j].Value }

// DoEvery execute function every d duration
func DoEvery(d time.Duration, f func()) {
	f()
	for range time.Tick(d) {
		f()
	}
}

// ArrToUpper uppercase every string in array
func ArrToUpper(arr []string) {
	for ii := range arr {
		arr[ii] = strings.ToUpper(arr[ii])
	}
}
