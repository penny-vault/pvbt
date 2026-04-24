package portfolio

import "github.com/penny-vault/pvbt/asset"

// SetAccountCurrentBatchID exposes Account.currentBatchID for tests.
func SetAccountCurrentBatchID(a *Account, id int) {
	a.currentBatchID = id
}

func GetAccountBatches(a *Account) []batchRecord {
	return a.batches
}

func GetAccountCurrentBatchID(a *Account) int {
	return a.currentBatchID
}

// PositionSeries returns the per-asset (mv, qty) history tracked by
// UpdatePrices. Test-only accessor.
func (a *Account) PositionSeries() (map[asset.Asset][]float64, map[asset.Asset][]float64) {
	return a.positionMV, a.positionQty
}
