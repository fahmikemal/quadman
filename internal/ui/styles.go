package ui

import "charm.land/lipgloss/v2"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("62")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	dimErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("174"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	filterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	lingerOnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	lingerOffStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	lingerUnknownStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)
)
