package cli

import (
	"fmt"
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penny-vault/pvbt/data"
)

// series holds the label and values for one (asset, metric) combination.
type series struct {
	label  string
	values []float64
	color  lipgloss.Color
}

// exploreGraphModel is a bubbletea Model that renders a multi-series
// text-based line chart for a DataFrame.
type exploreGraphModel struct {
	df     *data.DataFrame
	series []series
	width  int
	height int
}

// seriesColors defines the palette used for chart series.
var seriesColors = []lipgloss.Color{
	lipgloss.Color("12"), // blue
	lipgloss.Color("10"), // green
	lipgloss.Color("9"),  // red
	lipgloss.Color("11"), // yellow
	lipgloss.Color("13"), // magenta
	lipgloss.Color("14"), // cyan
	lipgloss.Color("15"), // white
	lipgloss.Color("208"), // orange
}

func newExploreGraphModel(df *data.DataFrame) exploreGraphModel {
	assets := df.AssetList()
	metrics := df.MetricList()

	var allSeries []series
	colorIdx := 0
	for _, a := range assets {
		for _, m := range metrics {
			col := df.Column(a, m)
			label := fmt.Sprintf("%s/%s", a.Ticker, string(m))
			allSeries = append(allSeries, series{
				label:  label,
				values: col,
				color:  seriesColors[colorIdx%len(seriesColors)],
			})
			colorIdx++
		}
	}

	return exploreGraphModel{
		df:     df,
		series: allSeries,
		width:  80,
		height: 24,
	}
}

func (m exploreGraphModel) Init() tea.Cmd {
	return nil
}

func (m exploreGraphModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m exploreGraphModel) View() string {
	if m.width == 0 || len(m.series) == 0 {
		return "Loading..."
	}

	// Layout: title (1) + chart + legend (series count + 1) + help (1)
	legendHeight := len(m.series) + 1
	chartHeight := m.height - 2 - legendHeight - 1 // 2 for title+blank, 1 for help
	if chartHeight < 3 {
		chartHeight = 3
	}
	multiSeries := len(m.series) > 1
	rightMargin := 11 // always show right pct axis for single; left pct axis for multi
	if multiSeries {
		rightMargin = 0
	}
	chartWidth := m.width - 10 - rightMargin // reserve space for left Y-axis + optional right pct axis
	if chartWidth < 10 {
		chartWidth = 10
	}

	// Title
	title := lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("15")).
		Render("Data Explorer")

	// Date range subtitle
	times := m.df.Times()
	dateRange := fmt.Sprintf("  %s to %s  (%d points)",
		times[0].Format("2006-01-02"),
		times[len(times)-1].Format("2006-01-02"),
		len(times))
	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("7")).
		Render(dateRange)

	// Compute first valid value per series (for percent-change conversion)
	firstVal := make([]float64, len(m.series))
	for si, s := range m.series {
		firstVal[si] = math.NaN()
		for _, v := range s.values {
			if !math.IsNaN(v) {
				firstVal[si] = v
				break
			}
		}
	}

	// For multi-series, convert values to percent change so they share a scale.
	// For single series, use raw values.
	plotSeries := make([][]float64, len(m.series))
	if multiSeries {
		for si, s := range m.series {
			pct := make([]float64, len(s.values))
			for i, v := range s.values {
				if math.IsNaN(v) || math.IsNaN(firstVal[si]) || firstVal[si] == 0 {
					pct[i] = math.NaN()
				} else {
					pct[i] = (v - firstVal[si]) / firstVal[si] * 100
				}
			}
			plotSeries[si] = pct
		}
	} else {
		for si, s := range m.series {
			plotSeries[si] = s.values
		}
	}

	// Compute global min/max across plot values
	minVal := math.Inf(1)
	maxVal := math.Inf(-1)
	for _, vals := range plotSeries {
		for _, v := range vals {
			if !math.IsNaN(v) && v < minVal {
				minVal = v
			}
			if !math.IsNaN(v) && v > maxVal {
				maxVal = v
			}
		}
	}
	if math.IsInf(minVal, 1) || math.IsInf(maxVal, -1) {
		minVal = 0
		maxVal = 1
	}
	valRange := maxVal - minVal
	if valRange == 0 {
		valRange = 1
	}

	// Build the chart grid: each cell holds the index of the series that
	// occupies it, or -1 if empty.
	grid := make([][]int, chartHeight)
	for r := range grid {
		grid[r] = make([]int, chartWidth)
		for c := range grid[r] {
			grid[r][c] = -1
		}
	}

	n := len(times)
	valToRow := func(v float64) int {
		row := int((maxVal - v) / valRange * float64(chartHeight-1))
		if row < 0 {
			row = 0
		}
		if row >= chartHeight {
			row = chartHeight - 1
		}
		return row
	}

	for si, vals := range plotSeries {
		prevRow := -1
		for col := 0; col < chartWidth; col++ {
			// Map chart column to data index
			idx := col * n / chartWidth
			if idx >= len(vals) {
				idx = len(vals) - 1
			}
			v := vals[idx]
			if math.IsNaN(v) {
				prevRow = -1
				continue
			}
			row := valToRow(v)

			// Draw a vertical line connecting previous point to current
			if prevRow >= 0 && prevRow != row {
				lo, hi := prevRow, row
				if lo > hi {
					lo, hi = hi, lo
				}
				for r := lo; r <= hi; r++ {
					if grid[r][col] == -1 {
						grid[r][col] = si
					}
				}
			} else {
				if grid[row][col] == -1 {
					grid[row][col] = si
				}
			}
			prevRow = row
		}
	}

	// Determine which rows get horizontal grid lines (every ~5 rows)
	gridInterval := chartHeight / 5
	if gridInterval < 2 {
		gridInterval = 2
	}
	isGridRow := func(row int) bool {
		return row == 0 || row == chartHeight-1 || row%gridInterval == 0
	}

	// For single series: pct change on right axis using first value as reference
	refVal := firstVal[0]
	pctForRow := func(row int) string {
		if math.IsNaN(refVal) || refVal == 0 {
			return ""
		}
		val := maxVal - float64(row)/float64(chartHeight-1)*valRange
		pct := (val - refVal) / refVal * 100
		return fmt.Sprintf("%+.1f%%", pct)
	}

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	gridStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("237"))

	// Render the chart
	var chartLines []string
	for row := 0; row < chartHeight; row++ {
		// Left Y-axis label
		yLabel := "        "
		if isGridRow(row) {
			val := maxVal - float64(row)/float64(chartHeight-1)*valRange
			if multiSeries {
				yLabel = fmt.Sprintf("%+7.1f%%", val)
			} else {
				yLabel = fmt.Sprintf("%8.2f", val)
			}
		}

		var sb strings.Builder
		sb.WriteString(dimStyle.Render(yLabel))
		if isGridRow(row) {
			sb.WriteString(dimStyle.Render("\u2524"))
		} else {
			sb.WriteString(dimStyle.Render("\u2502"))
		}

		for col := 0; col < chartWidth; col++ {
			si := grid[row][col]
			if si >= 0 {
				style := lipgloss.NewStyle().Foreground(m.series[si].color)
				sb.WriteString(style.Render("\u2588"))
			} else if isGridRow(row) {
				sb.WriteString(gridStyle.Render("\u2500"))
			} else {
				sb.WriteString(" ")
			}
		}

		// Right Y-axis with percent change (only for single series)
		if !multiSeries {
			if isGridRow(row) {
				sb.WriteString(dimStyle.Render("\u251c"))
				sb.WriteString(dimStyle.Render(fmt.Sprintf(" %-8s", pctForRow(row))))
			} else {
				sb.WriteString(dimStyle.Render("\u2502"))
			}
		}

		chartLines = append(chartLines, sb.String())
	}

	// X-axis: show start and end dates
	xAxis := strings.Repeat(" ", 9) +
		times[0].Format("2006-01-02") +
		strings.Repeat(" ", max(0, chartWidth-20)) +
		times[len(times)-1].Format("2006-01-02")

	// Legend
	var legendParts []string
	for _, s := range m.series {
		swatch := lipgloss.NewStyle().Foreground(s.color).Render("\u2588\u2588")
		legendParts = append(legendParts, fmt.Sprintf("  %s %s", swatch, s.label))
	}

	// Help
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).
		Render("  Press q or esc to quit")

	// Assemble
	var parts []string
	parts = append(parts, title+"  "+subtitle)
	parts = append(parts, "")
	parts = append(parts, chartLines...)
	parts = append(parts, xAxis)
	parts = append(parts, "")
	parts = append(parts, legendParts...)
	parts = append(parts, help)

	return strings.Join(parts, "\n")
}

// runExploreGraph creates and runs the TUI graph program.
func runExploreGraph(df *data.DataFrame) error {
	model := newExploreGraphModel(df)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
