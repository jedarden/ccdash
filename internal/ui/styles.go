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
