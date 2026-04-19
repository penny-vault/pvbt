package portfolio

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
