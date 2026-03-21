package portfolio

// LotSelection determines which tax lots are consumed when selling a position.
type LotSelection int

const (
	// LotFIFO sells the earliest-acquired lots first (default).
	LotFIFO LotSelection = iota
	// LotLIFO sells the most-recently-acquired lots first.
	LotLIFO
	// LotHighestCost sells the lot with the highest cost basis first,
	// producing the largest realized loss when the position is underwater.
	LotHighestCost
	// LotSpecificID sells a specific lot identified by ID.
	LotSpecificID
)
