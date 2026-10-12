package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// This file holds the Bubble Tea messages exchanged between commands and
// Update, plus the armed-confirmation type. Handlers live in update.go.

type actionMsg struct {
	desc string
	hint string
	out  string
	err  error
	took time.Duration
	// run is the work itself, kept so the actions screen can repeat it.
	run func(context.Context) (string, error)
	// epoch is the session the action ran against; guard is the
	// confirmation it passed (nil when it needed none).
	epoch uint64
	guard *confirmGuard
}

// confirmGuard is the confirmation level an action passed: repeating it
// must ask the same way (danger tint, typed name), never with a bare y.
type confirmGuard struct {
	title  string
	lines  []string
	keeps  []string
	danger bool
	typed  string
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
// confirmation in the confirm modal.
type pendingAction struct {
	verb string // "stop", "enable", "disable", "bulk-stop", "delete", …
	unit quadlet.Unit

	// Optional modal content; empty fields fall back to the verb's
	// built-in wording (confirm.go).
	title  string
	lines  []string // THIS WILL: what happens
	keeps  []string // KEEPS: what is left untouched
	danger bool     // err-tinted modal instead of warn
	typed  string   // the exact text the user must type to enable Confirm
	// run performs a custom action; nil means the verb's built-in action.
	run func(Model) (tea.Model, tea.Cmd)
}
