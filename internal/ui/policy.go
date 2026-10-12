package ui

// This file is quadman's single capability policy. Every state-changing
// or privileged action declares what it needs from the session; the
// policy decides, for each session kind (local, SSH, compartment, served
// over Wish) and the readonly switch, whether it may run. Key handlers
// call refuse, palette entries call allowed — so both surfaces always
// agree, and a new action cannot forget a gate: it has to name its
// capabilities to run at all.

// capability is one kind of power an action needs from the session.
type capability uint8

const (
	// capMutate changes runtime state on the target: lifecycle, reload,
	// linger, secrets, storage, timers, updates, custom commands, exec.
	capMutate capability = 1 << iota
	// capTargetFiles writes or deletes Quadlet files through quadman's own
	// file I/O (boot toggles, delete, generate, editor). That I/O happens
	// on the operator's filesystem, so it is refused in SSH and
	// compartment sessions, where the files are not the operator's.
	capTargetFiles
	// capLocalFiles writes a file on the machine quadman runs on (log
	// export). A served session must never write into the daemon's cwd.
	capLocalFiles
	// capTerminal hands the terminal to a child process (editor, exec
	// shell). The SSH runner is batch-only and a served session has no
	// local terminal to hand over.
	capTerminal
	// capSecretValue reveals a secret's plaintext (clipboard copy).
	capSecretValue
	// capSwitchSession re-targets the session (compartments, own session).
	capSwitchSession
)

// refusal explains why the policy denied an action: short for palette
// rows, long for the status line.
type refusal struct {
	short, long string
}

const (
	servedMsg   = "unavailable in served sessions: connect to the host with ssh and run quadman there"
	sshTTYMsg   = "unavailable over SSH: an interactive terminal cannot cross the batch runner (ssh to the host and run it there)"
	pinnedMsg   = "this session stays on the target it was started for"
	systemComps = "compartments are user sessions; unavailable in system mode"
)

// isServed reports a session served by `quadman serve` (Wish SSH).
func (m Model) isServed() bool { return m.clientInfo != "" || m.noEditor }

// check applies the policy to caps. ok is true when the action may run.
func (m Model) check(caps capability) (refusal, bool) {
	if caps == 0 {
		return refusal{}, true
	}
	if m.readonly {
		switch {
		case caps&capSecretValue != 0:
			return refusal{"readonly", "copying secret values is disabled in readonly and served sessions"}, false
		case caps == capLocalFiles:
			return refusal{"readonly", "readonly mode: export to a file is refused (c copies the lines to the clipboard)"}, false
		}
		return refusal{"readonly", readonlyMsg}, false
	}
	if m.isServed() {
		switch {
		case caps&capSecretValue != 0:
			return refusal{"served session", "copying secret values is disabled in readonly and served sessions"}, false
		case caps&capSwitchSession != 0:
			return refusal{"served session", "unavailable in served sessions: " + pinnedMsg}, false
		case caps&(capTerminal|capLocalFiles) != 0:
			return refusal{"served session", servedMsg}, false
		}
	}
	if caps&capTargetFiles != 0 {
		switch {
		case m.ssh.IsRemote():
			return refusal{"unavailable over SSH", remoteWriteMsg}, false
		case m.compOn:
			return refusal{"unavailable in compartment", "not available in compartment " + m.comp.User + ": files belong to " + m.comp.User + ", not the operator"}, false
		}
	}
	if caps&capTerminal != 0 && m.ssh.IsRemote() {
		return refusal{"unavailable over SSH", sshTTYMsg}, false
	}
	if caps&capSwitchSession != 0 {
		switch {
		case m.ssh.IsRemote():
			return refusal{"unavailable over SSH", "compartments are local sudo targets; unavailable over SSH (no nested hops)"}, false
		case m.system:
			return refusal{"unavailable in system mode", systemComps}, false
		}
	}
	return refusal{}, true
}

// allowed is the palette form of check: whether the action may run and
// the short reason when not.
func (m Model) allowed(caps capability) (bool, string) {
	r, ok := m.check(caps)
	return ok, r.short
}

// refuse is the key-handler form of check: it reports whether the action
// is refused, putting the full explanation in the status line when so.
func (m *Model) refuse(caps capability) bool {
	r, ok := m.check(caps)
	if !ok {
		m.setStatus(r.long, true)
	}
	return !ok
}
