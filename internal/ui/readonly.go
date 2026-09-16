package ui

// readonlyMsg is the explanation shown when a state-changing action is
// refused in readonly mode.
const readonlyMsg = "readonly mode: state-changing actions are disabled (restart without --readonly to manage units)"

// refuseReadonly reports whether the model is readonly, setting the status
// explanation when it is. Call at the top of every state-changing key
// handler.
func (m *Model) refuseReadonly() bool {
	if !m.readonly {
		return false
	}
	m.setStatus(readonlyMsg, true)
	return true
}
