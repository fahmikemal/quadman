package ui

import "github.com/fahmikemal/quadman/internal/quadlet"

// bootTarget is the target "enable at boot" wires a unit into: the user
// manager's default.target, or multi-user.target for system units.
func (m Model) bootTarget() string {
	if m.system {
		return "multi-user.target"
	}
	return "default.target"
}

// containerName is the name of u's container, resolved the same way
// everywhere (health, healthcheck, inspect, exec, events, stats): the
// running container's real name from Quadlet's PODMAN_SYSTEMD_UNIT label,
// else the file's ContainerName= with specifiers expanded, else Quadlet's
// systemd-%N default for the generated unit (ServiceName= aware).
func (m Model) containerName(u quadlet.Unit) string {
	if n := m.ctrNames[u.UnitName]; n != "" {
		return n
	}
	return quadlet.ContainerName(u.UnitName, m.ctrOverride[u.UnitName])
}

// containerHealth is u's healthcheck state ("healthy", "unhealthy",
// "starting"), "" without a healthcheck or a running container.
func (m Model) containerHealth(u quadlet.Unit) string {
	return m.health[m.containerName(u)]
}
