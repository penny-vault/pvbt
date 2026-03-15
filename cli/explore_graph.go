package cli

import (
	"fmt"
	"math"
	"strings"

	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/canvas/graph"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penny-vault/pvbt/data"
)

// chartSeries holds the label and values for one (asset, metric) combination.
type chartSeries struct {
	label  string
	values []float64
	color  lipgloss.Color
}

// exploreGraphModel is a bubbletea Model that renders a multi-series
// line chart for a DataFrame using ntcharts line-drawing characters.
type exploreGraphModel struct {
	df     *data.DataFrame
	series []chartSeries
	width  int
	height int
}

// seriesColors defines the palette used for chart series.
var seriesColors = []lipgloss.Color{
	lipgloss.Color("12"),  // blue
	lipgloss.Color("10"),  // green
	lipgloss.Color("9"),   // red
	lipgloss.Color("11"),  // yellow
	lipgloss.Color("13"),  // magenta
	lipgloss.Color("14"),  // cyan
	lipgloss.Color("15"),  // white
	lipgloss.Color("208"), // orange
}

func newExploreGraphModel(df *data.DataFrame) exploreGraphModel {
	assets := df.AssetList()
	metrics := df.MetricList()

	var allSeries []chartSeries

	colorIdx := 0

	for _, a := range assets {
		for _, m := range metrics {
			col := df.Column(a, m)
			label := fmt.Sprintf("%s/%s", a.Ticker, string(m))
			allSeries = append(allSeries, chartSeries{
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

	multiSeries := len(m.series) > 1

	// Layout: title (1) + blank (1) + chart + x-axis (1) + blank (1) + legend + help (1)
	legendHeight := len(m.series) + 1

	chartHeight := m.height - 2 - 1 - legendHeight - 1
	if chartHeight < 3 {
		chartHeight = 3
	}

	// Reserve space for left Y-axis labels and optional right pct axis
	leftMargin := 10 // "  nnn.nn "  + axis char

	rightMargin := 0
	if !multiSeries {
		rightMargin = 11
	}

	chartWidth := m.width - leftMargin - rightMargin
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
	for seriesIdx, s := range m.series {
		firstVal[seriesIdx] = math.NaN()

		for _, value := range s.values {
			if !math.IsNaN(value) {
				firstVal[seriesIdx] = value
				break
			}
		}
	}

	// For multi-series, convert to percent change so they share a scale.
	plotSeries := make([][]float64, len(m.series))
	if multiSeries {
		for seriesIdx, s := range m.series {
			pct := make([]float64, len(s.values))
			for ii, value := range s.values {
				if math.IsNaN(value) || math.IsNaN(firstVal[seriesIdx]) || firstVal[seriesIdx] == 0 {
					pct[ii] = math.NaN()
				} else {
					pct[ii] = (value - firstVal[seriesIdx]) / firstVal[seriesIdx] * 100
				}
			}

			plotSeries[seriesIdx] = pct
		}
	} else {
		for seriesIdx, s := range m.series {
			plotSeries[seriesIdx] = s.values
		}
	}

	// Compute global min/max across plot values
	minVal := math.Inf(1)
	maxVal := math.Inf(-1)

	for _, vals := range plotSeries {
		for _, value := range vals {
			if !math.IsNaN(value) && value < minVal {
				minVal = value
			}

			if !math.IsNaN(value) && value > maxVal {
				maxVal = value
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

	// Create an ntcharts canvas for the chart area
	chart := canvas.New(chartWidth, chartHeight)

	// Map data to canvas Y coordinates and draw each series
	numPoints := len(times)
	xAxisRow := chartHeight - 1 // x-axis at bottom of canvas

	for seriesIdx, vals := range plotSeries {
		// Resample data to chart width
		seqY := make([]int, chartWidth)

		hasData := make([]bool, chartWidth)
		for col := 0; col < chartWidth; col++ {
			idx := col * numPoints / chartWidth
			if idx >= len(vals) {
				idx = len(vals) - 1
			}

			value := vals[idx]
			if math.IsNaN(value) {
				hasData[col] = false
				continue
			}
			// Map value to canvas Y: cartesian Y where 0=bottom, chartHeight-1=top
			cartY := int((value - minVal) / valRange * float64(chartHeight-1))
			if cartY < 0 {
				cartY = 0
			}

			if cartY >= chartHeight {
				cartY = chartHeight - 1
			}
			// Convert to canvas coordinates (0,0 is top-left)
			seqY[col] = canvas.CanvasYCoordinate(xAxisRow, cartY)
			hasData[col] = true
		}

		// Build contiguous segments (skip NaN gaps)
		style := lipgloss.NewStyle().Foreground(m.series[seriesIdx].color)
		startCol := -1

		for col := 0; col <= chartWidth; col++ {
			if col < chartWidth && hasData[col] {
				if startCol < 0 {
					startCol = col
				}
			} else {
				if startCol >= 0 {
					// Draw this segment
					seg := seqY[startCol:col]
					graph.DrawLineSequence(&chart, false, startCol, seg, runes.ArcLineStyle, style)
					startCol = -1
				}
			}
		}
	}

	// Draw horizontal grid lines
	gridInterval := chartHeight / 5
	if gridInterval < 2 {
		gridInterval = 2
	}

	gridStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("237"))

	for row := 0; row < chartHeight; row++ {
		if row == 0 || row == chartHeight-1 || row%gridInterval == 0 {
			for col := 0; col < chartWidth; col++ {
				cell := chart.Cell(canvas.Point{X: col, Y: row})
				if cell.Rune == 0 || cell.Rune == ' ' {
					chart.SetRuneWithStyle(canvas.Point{X: col, Y: row}, '\u2500', gridStyle)
				}
			}
		}
	}

	// Get the canvas view (string with newlines)
	canvasView := chart.View()
	canvasLines := strings.Split(canvasView, "\n")

	// Determine grid row positions for Y-axis labels
	isGridRow := func(row int) bool {
		return row == 0 || row == chartHeight-1 || row%gridInterval == 0
	}

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))

	// For single series: pct change on right axis
	refVal := firstVal[0]
	pctForRow := func(row int) string {
		if math.IsNaN(refVal) || refVal == 0 {
			return ""
		}

		val := maxVal - float64(row)/float64(chartHeight-1)*valRange
		pct := (val - refVal) / refVal * 100

		return fmt.Sprintf("%+.1f%%", pct)
	}

	// Combine Y-axis labels with canvas lines
	var chartLines []string

	for row := 0; row < chartHeight && row < len(canvasLines); row++ {
		var sb strings.Builder

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

		sb.WriteString(dimStyle.Render(yLabel))

		if isGridRow(row) {
			sb.WriteString(dimStyle.Render("\u2524"))
		} else {
			sb.WriteString(dimStyle.Render("\u2502"))
		}

		// Canvas row
		sb.WriteString(canvasLines[row])

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
	xAxis := strings.Repeat(" ", leftMargin) +
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
