package portfolio

// SetAccountCurrentBatchID exposes Account.currentBatchID for tests.
func SetAccountCurrentBatchID(a *Account, id int) {
	a.currentBatchID = id
}
