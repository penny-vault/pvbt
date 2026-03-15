package cli

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog"
)

// -- messages ----------------------------------------------------------------

type tickMsg struct {
	date  time.Time
	value float64
}

type logMsg string

type progressMsg struct {
	current int
	total   int
}

type doneMsg struct{}

// -- tuiModel ----------------------------------------------------------------

type tuiModel struct {
	// equity curve data
	equityDates  []time.Time
	equityValues []float64

	// metrics
	portfolioValue float64
	totalReturn    float64
	sharpe         float64
	sortino        float64
	maxDrawdown    float64
	beta           float64
	alpha          float64

	// logs
	logs    []string
	maxLogs int

	// progress
	current int
	total   int
	done    bool

	// layout
	width  int
	height int
}

func newTUIModel() tuiModel {
	return tuiModel{
		maxLogs: 100,
		width:   80,
		height:  24,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tickMsg:
		m.equityDates = append(m.equityDates, msg.date)
		m.equityValues = append(m.equityValues, msg.value)

		m.portfolioValue = msg.value
		if len(m.equityValues) > 1 && m.equityValues[0] != 0 {
			m.totalReturn = (msg.value - m.equityValues[0]) / m.equityValues[0]
		}
	case logMsg:
		m.logs = append(m.logs, string(msg))
		if len(m.logs) > m.maxLogs {
			m.logs = m.logs[len(m.logs)-m.maxLogs:]
		}
	case progressMsg:
		m.current = msg.current
		m.total = msg.total
	case doneMsg:
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

func (m tuiModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	metricsWidth := 28

	chartWidth := m.width - metricsWidth - 3
	if chartWidth < 20 {
		chartWidth = 20
	}

	logHeight := 6
	progressHeight := 1

	chartHeight := m.height - logHeight - progressHeight - 4
	if chartHeight < 5 {
		chartHeight = 5
	}

	// equity curve (simple sparkline)
	chart := m.renderChart(chartWidth, chartHeight)

	// metrics sidebar
	metrics := m.renderMetrics(metricsWidth, chartHeight)

	// top section: chart + metrics side by side
	topSection := lipgloss.JoinHorizontal(lipgloss.Top, chart, metrics)

	// logs section (full width)
	logs := m.renderLogs(m.width, logHeight)

	// progress bar (full width)
	progress := m.renderProgress(m.width)

	return lipgloss.JoinVertical(lipgloss.Left, topSection, logs, progress)
}

func (m tuiModel) renderChart(width, height int) string {
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(width).
		Height(height)

	title := lipgloss.NewStyle().Bold(true).Render("Equity Curve")

	if len(m.equityValues) < 2 {
		return border.Render(title + "\n\n  Waiting for data...")
	}

	// simple text-based chart
	minVal, maxVal := m.equityValues[0], m.equityValues[0]
	for _, equity := range m.equityValues {
		if equity < minVal {
			minVal = equity
		}

		if equity > maxVal {
			maxVal = equity
		}
	}

	chartH := height - 2
	if chartH < 1 {
		chartH = 1
	}

	chartW := width - 2
	if chartW < 1 {
		chartW = 1
	}

	valRange := maxVal - minVal
	if valRange == 0 {
		valRange = 1
	}

	// downsample to fit width
	step := len(m.equityValues) / chartW
	if step < 1 {
		step = 1
	}

	var points []float64
	for i := 0; i < len(m.equityValues); i += step {
		points = append(points, m.equityValues[i])
	}

	if len(points) > chartW {
		points = points[:chartW]
	}

	// render rows
	lines := make([]string, chartH)
	for row := 0; row < chartH; row++ {
		threshold := maxVal - (float64(row)/float64(chartH-1))*valRange

		var sb strings.Builder

		for _, p := range points {
			if p >= threshold {
				sb.WriteRune('\u2588')
			} else {
				sb.WriteRune(' ')
			}
		}

		lines[row] = sb.String()
	}

	content := title + "\n" + strings.Join(lines, "\n")

	return border.Render(content)
}

func (m tuiModel) renderMetrics(width, height int) string {
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(width).
		Height(height)

	label := lipgloss.NewStyle().Width(18).Foreground(lipgloss.Color("7"))
	value := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Render("Metrics"))
	sb.WriteString("\n\n")
	sb.WriteString(label.Render("Value:"))
	sb.WriteString(value.Render(fmt.Sprintf("$%.0f", m.portfolioValue)))
	sb.WriteString("\n")
	sb.WriteString(label.Render("Return:"))
	sb.WriteString(value.Render(fmt.Sprintf("%.1f%%", m.totalReturn*100)))
	sb.WriteString("\n")
	sb.WriteString(label.Render("Sharpe:"))
	sb.WriteString(value.Render(fmt.Sprintf("%.2f", m.sharpe)))
	sb.WriteString("\n")
	sb.WriteString(label.Render("Sortino:"))
	sb.WriteString(value.Render(fmt.Sprintf("%.2f", m.sortino)))
	sb.WriteString("\n")
	sb.WriteString(label.Render("MaxDD:"))
	sb.WriteString(value.Render(fmt.Sprintf("%.1f%%", m.maxDrawdown*100)))
	sb.WriteString("\n")
	sb.WriteString(label.Render("Beta:"))
	sb.WriteString(value.Render(fmt.Sprintf("%.2f", m.beta)))
	sb.WriteString("\n")
	sb.WriteString(label.Render("Alpha:"))
	sb.WriteString(value.Render(fmt.Sprintf("%.1f%%", m.alpha*100)))

	return border.Render(sb.String())
}

func (m tuiModel) renderLogs(width, height int) string {
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(width - 2).
		Height(height)

	title := lipgloss.NewStyle().Bold(true).Render("Logs")

	visible := m.logs
	if len(visible) > height-1 {
		visible = visible[len(visible)-(height-1):]
	}

	content := title + "\n" + strings.Join(visible, "\n")

	return border.Render(content)
}

func (m tuiModel) renderProgress(width int) string {
	if m.total == 0 {
		return ""
	}

	pct := float64(m.current) / float64(m.total)

	barWidth := width - 20
	if barWidth < 10 {
		barWidth = 10
	}

	filled := int(pct * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("=", filled) + strings.Repeat("-", barWidth-filled)

	return fmt.Sprintf(" %s  %3.0f%%  %d/%d", bar, pct*100, m.current, m.total)
}

// -- zerolog writer adapter --------------------------------------------------

// newTUILogWriter creates a zerolog.LevelWriter that sends log messages to the TUI.
func newTUILogWriter(p *tea.Program) zerolog.LevelWriter {
	return &tuiLevelWriter{program: p}
}

type tuiLevelWriter struct {
	program *tea.Program
}

func (w *tuiLevelWriter) Write(buf []byte) (n int, err error) {
	msg := strings.TrimRight(string(buf), "\n")
	if w.program != nil {
		w.program.Send(logMsg(msg))
	}

	return len(buf), nil
}

func (w *tuiLevelWriter) WriteLevel(_ zerolog.Level, buf []byte) (n int, err error) {
	return w.Write(buf)
}
