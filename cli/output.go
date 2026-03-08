package cli

import (
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/portfolio"
)

func writePortfolio(base, ext, runID, strategy string, start, end time.Time, cash float64, acct *portfolio.Account) error {
	switch ext {
	case ".jsonl":
		return writePortfolioJSONL(base+ext, runID, strategy, start, end, cash, acct)
	case ".parquet":
		return writePortfolioParquet(base+ext, runID, strategy, start, end, cash, acct)
	default:
		return fmt.Errorf("unsupported output format: %s", ext)
	}
}

func writeTransactions(base, ext string, acct *portfolio.Account) error {
	path := base + "-transactions" + ext
	switch ext {
	case ".jsonl":
		return writeTransactionsJSONL(path, acct)
	case ".parquet":
		return writeTransactionsParquet(path, acct)
	default:
		return fmt.Errorf("unsupported output format: %s", ext)
	}
}

func writeHoldings(base, ext string, acct *portfolio.Account) error {
	path := base + "-holdings" + ext
	switch ext {
	case ".jsonl":
		return writeHoldingsJSONL(path, acct)
	case ".parquet":
		return writeHoldingsParquet(path, acct)
	default:
		return fmt.Errorf("unsupported output format: %s", ext)
	}
}

func writeMetrics(base, ext string, acct *portfolio.Account) error {
	path := base + "-metrics" + ext
	switch ext {
	case ".jsonl":
		return writeMetricsJSONL(path, acct)
	case ".parquet":
		return writeMetricsParquet(path, acct)
	default:
		return fmt.Errorf("unsupported output format: %s", ext)
	}
}
