package engine

import (
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/tradecron"
)

// walkBackTradingDays finds the trading date that is `days` trading days
// before `from`. It uses a forward-walk approach since TradeCron only
// supports Next(). Returns an error if days is negative.
func walkBackTradingDays(from time.Time, days int) (time.Time, error) {
	if days < 0 {
		return time.Time{}, fmt.Errorf("walkBackTradingDays: days must be non-negative, got %d", days)
	}

	if days == 0 {
		return from, nil
	}

	daily, err := tradecron.New("@close * * *", tradecron.RegularHours)
	if err != nil {
		return time.Time{}, fmt.Errorf("walkBackTradingDays: creating daily schedule: %w", err)
	}

	// Estimate calendar days needed. Start with 2x multiplier to account
	// for weekends and holidays. Retry with doubled offset up to 3 times.
	multiplier := 2
	const maxAttempts = 3

	for attempt := range maxAttempts {
		calendarDays := days * multiplier * (1 << attempt) // 2x, 4x, 8x
		estimatedStart := from.AddDate(0, 0, -calendarDays)

		// Walk forward from estimated start, collecting trading days.
		var tradingDays []time.Time
		cur := daily.Next(estimatedStart.Add(-time.Nanosecond))

		for !cur.After(from) {
			tradingDays = append(tradingDays, cur)
			cur = daily.Next(cur.Add(time.Nanosecond))
		}

		if len(tradingDays) >= days+1 {
			// We have enough trading days. The target is at index
			// len(tradingDays) - 1 - days (counting back from `from`).
			targetIdx := len(tradingDays) - 1 - days
			return tradingDays[targetIdx], nil
		}
	}

	return time.Time{}, fmt.Errorf("walkBackTradingDays: could not find %d trading days before %s after %d attempts",
		days, from.Format("2006-01-02"), maxAttempts)
}
