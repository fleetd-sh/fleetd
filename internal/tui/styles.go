package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	primary   = lipgloss.Color("#7D56F4")
	secondary = lipgloss.Color("#56C7F4")
	success   = lipgloss.Color("#00D787")
	warning   = lipgloss.Color("#FFB86C")
	danger    = lipgloss.Color("#FF5555")
	muted     = lipgloss.Color("#6272A4")
	text      = lipgloss.Color("#F8F8F2")

	// Base styles
	BaseStyle = lipgloss.NewStyle().
			Padding(1, 2)

	// Title styles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primary).
			MarginBottom(1)

	// Card styles
	CardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primary).
			Padding(1, 2).
			MarginBottom(1)

	// Status badge styles
	StatusRunning = lipgloss.NewStyle().
			Background(success).
			Foreground(lipgloss.Color("#000")).
			Padding(0, 1).
			Bold(true)

	StatusStopped = lipgloss.NewStyle().
			Background(muted).
			Foreground(text).
			Padding(0, 1)

	StatusError = lipgloss.NewStyle().
			Background(danger).
			Foreground(text).
			Padding(0, 1).
			Bold(true)

	StatusPending = lipgloss.NewStyle().
			Background(warning).
			Foreground(lipgloss.Color("#000")).
			Padding(0, 1)

	// Log styles
	LogStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(secondary).
		Padding(1, 2).
		Height(10)

	LogInfoStyle = lipgloss.NewStyle().
			Foreground(secondary)

	LogSuccessStyle = lipgloss.NewStyle().
			Foreground(success)

	LogWarningStyle = lipgloss.NewStyle().
			Foreground(warning)

	LogErrorStyle = lipgloss.NewStyle().
			Foreground(danger)

	// Button styles
	ButtonActive = lipgloss.NewStyle().
			Background(primary).
			Foreground(text).
			Padding(0, 3).
			Bold(true)

	ButtonInactive = lipgloss.NewStyle().
			Background(muted).
			Foreground(text).
			Padding(0, 3)

	// Help text
	HelpStyle = lipgloss.NewStyle().
			Foreground(muted).
			MarginTop(1)
)
