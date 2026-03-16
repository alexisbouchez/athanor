package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Status styles
	stylePending = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	stylePassed  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleSkipped = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)

	// Panel styles
	stylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("5")).
			Padding(0, 1)

	styleLogLine = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	// Glyphs
	glyphPending = stylePending.Render("·")
	glyphRunning = styleRunning.Render("●")
	glyphPassed  = stylePassed.Render("✓")
	glyphFailed  = styleFailed.Render("✗")
	glyphSkipped = styleSkipped.Render("○")
)
