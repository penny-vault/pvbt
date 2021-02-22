package util

import (
	"strings"
	"time"
)

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
