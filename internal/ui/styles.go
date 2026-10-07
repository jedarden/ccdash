package ui

import "github.com/charmbracelet/lipgloss"

// Styles defines the styling for the UI components
var (
	// TitleStyle is used for panel titles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	// PanelStyle is used for panel borders
	PanelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))
)

// Styles (unified-dashboard palette)
var (
	panelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00aaff")).
			Padding(0, 1) // Minimal padding for compact display

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00ffff")).
			MarginBottom(1)

	boldStyle = lipgloss.NewStyle().
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00ff00"))

	askingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff00ff")).
			Bold(true)

	costStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffaa00")).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffaa00"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff0000")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00ffff")).
			Background(lipgloss.Color("#1a1a1a")).
			Padding(0, 1)
)
