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
	chartWidth := m.width - 10 // reserve space for Y-axis labels
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

	// Compute global min/max across all series
	minVal := math.Inf(1)
	maxVal := math.Inf(-1)
	for _, s := range m.series {
		for _, v := range s.values {
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
	for si, s := range m.series {
		for col := 0; col < chartWidth; col++ {
			// Map chart column to data index
			idx := col * n / chartWidth
			if idx >= len(s.values) {
				idx = len(s.values) - 1
			}
			v := s.values[idx]
			if math.IsNaN(v) {
				continue
			}
			// Map value to row (row 0 = top = maxVal)
			row := int((maxVal - v) / valRange * float64(chartHeight-1))
			if row < 0 {
				row = 0
			}
			if row >= chartHeight {
				row = chartHeight - 1
			}
			// Only overwrite if empty (earlier series get priority)
			if grid[row][col] == -1 {
				grid[row][col] = si
			}
		}
	}

	// Render the chart with Y-axis labels
	var chartLines []string
	for row := 0; row < chartHeight; row++ {
		// Y-axis label: show at top, middle, and bottom
		yLabel := "        "
		switch {
		case row == 0:
			yLabel = fmt.Sprintf("%8.2f", maxVal)
		case row == chartHeight-1:
			yLabel = fmt.Sprintf("%8.2f", minVal)
		case row == chartHeight/2:
			yLabel = fmt.Sprintf("%8.2f", minVal+valRange/2)
		}

		var sb strings.Builder
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Render(yLabel))
		sb.WriteString(" ")

		for col := 0; col < chartWidth; col++ {
			si := grid[row][col]
			if si >= 0 {
				style := lipgloss.NewStyle().Foreground(m.series[si].color)
				sb.WriteString(style.Render("\u2588"))
			} else {
				sb.WriteString(" ")
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
