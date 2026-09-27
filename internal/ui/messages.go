package ui

import (
	"github.com/fahmikemal/quadman/internal/quadlet"
)

// This file holds the Bubble Tea messages exchanged between commands and
// Update, plus the armed-confirmation type. Handlers live in update.go.

type actionMsg struct {
	desc string
	hint string
	out  string
	err  error
}

type logsMsg struct {
	unit string
	out  string
	err  error
}

type lingerSetMsg struct {
	on    bool
	known bool
	err   error
}

type statusMsg struct {
	content string
}

type inspectMsg struct {
	content string
}

type editorFinishedMsg struct {
	changed bool
	err     error
}

type healthMsg struct {
	container string
	ok        bool
	err       error
}

// pendingAction is an armed destructive/config-changing action waiting for
// a y/N confirmation.
type pendingAction struct {
	verb string // "stop", "enable", "disable"
	unit quadlet.Unit
}
