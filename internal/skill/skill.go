// Package skill provides an exportable Agent Skill and tool definition
// for LLM coding agents (e.g. Antigravity, Gemini, Claude, ChatGPT, Codex)
// to understand, inspect, and orchestrate Podman Quadlets via quadman.
package skill

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Definition represents the structured metadata of the quadman agent skill.
type Definition struct {
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Description string       `json:"description"`
	Commands    []CommandDoc `json:"commands"`
	QuadletInfo QuadletDoc   `json:"quadlet_spec"`
	ToolSchema  []ToolDef    `json:"tools"`
}

// CommandDoc documents one CLI command or flag mode.
type CommandDoc struct {
	Usage       string `json:"usage"`
	Description string `json:"description"`
	Mode        string `json:"mode,omitempty"`
}

// QuadletDoc documents supported Quadlet unit kinds and search paths.
type QuadletDoc struct {
	FileExtensions []string `json:"file_extensions"`
	SearchPaths    []string `json:"search_paths"`
	Directives     []string `json:"common_directives"`
}

// ToolDef represents a machine-readable tool schema for AI function calling.
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Get returns the canonical Agent Skill definition for quadman.
func Get(ver string) Definition {
	return Definition{
		Name:        "quadman",
		Version:     ver,
		Description: "Terminal UI manager and CLI tool for rootless Podman Quadlet units & systemd lifecycle management. Operates without podman socket requirements via direct systemctl/journalctl/loginctl and Quadlet generator integration.",
		Commands: []CommandDoc{
			{Usage: "quadman", Description: "Launch full interactive Terminal UI in rootless user session."},
			{Usage: "quadman --system", Description: "Manage system-wide (rootful) Quadlet units (/etc/containers/systemd)."},
			{Usage: "sudo quadman", Description: "Auto-detects root (UID 0) and switches to system-wide mode."},
			{Usage: "quadman list", Description: "Non-interactive tab-delimited overview of all Quadlet source files and systemd states (for scripts/pipes)."},
			{Usage: "quadman list --system", Description: "List system-wide Quadlet units non-interactively."},
			{Usage: "quadman serve [-p 2222] [--readonly]", Description: "Run embedded Wish v2 SSH daemon to serve TUI over network."},
			{Usage: "quadman --ssh user@host", Description: "Transparently manage remote rootless Quadlets over SSH client."},
			{Usage: "quadman --readonly", Description: "Launch TUI with all state-changing actions disabled (monitoring mode)."},
			{Usage: "quadman --skill [format]", Description: "Export Agent Skill definition (format: markdown or json)."},
		},
		QuadletInfo: QuadletDoc{
			FileExtensions: []string{
				".container (single container)",
				".pod (pod definition)",
				".volume (persistent volume)",
				".network (Netavark/CNI network)",
				".image (image pull spec)",
				".build (Containerfile build spec)",
				".artifact (OCI artifact)",
				".kube (Kubernetes YAML via podman kube play)",
				".quadlets (multi-document bundle)",
			},
			SearchPaths: []string{
				"$XDG_RUNTIME_DIR/containers/systemd (highest priority, ephemeral)",
				"~/.config/containers/systemd (standard rootless user path)",
				"/etc/containers/systemd/users/ (admin-defined rootless units)",
				"/etc/containers/systemd/ (system-wide units in --system mode)",
				"/usr/share/containers/systemd/ (vendor-defined units)",
			},
			Directives: []string{
				"Image= (container image reference)",
				"PublishPort= (port mappings, e.g. 8080:80)",
				"Volume= (volume mounts, e.g. data.volume:/data:Z)",
				"Network= (network attachment, e.g. intra.network)",
				"Secret= (secret injection, e.g. secret_name,type=env,target=VAR)",
				"AutoUpdate=registry|local (auto-update configuration)",
				"ImageVolume=anonymous|tmpfs (Podman 6.x volume control)",
				"[Install] WantedBy=default.target (enable container at boot)",
			},
		},
		ToolSchema: []ToolDef{
			{
				Name:        "quadman_list",
				Description: "Query all Quadlet units and their active systemd states.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"system": map[string]interface{}{
							"type":        "boolean",
							"description": "Set to true to query system-wide units instead of user rootless units.",
						},
					},
				},
			},
			{
				Name:        "quadman_validate",
				Description: "Dry-run validate Quadlet source files using the Podman system generator and Secret references.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"system": map[string]interface{}{
							"type":        "boolean",
							"description": "Set to true to validate system-wide directories.",
						},
					},
				},
			},
		},
	}
}

// Markdown formats the definition as a comprehensive Markdown agent skill document.
func (d Definition) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Agent Skill: %s (v%s)\n\n", d.Name, d.Version)
	fmt.Fprintf(&b, "%s\n\n", d.Description)

	b.WriteString("## Command CLI Invocations\n\n")
	b.WriteString("| Command | Description |\n| :--- | :--- |\n")
	for _, cmd := range d.Commands {
		fmt.Fprintf(&b, "| `%s` | %s |\n", cmd.Usage, cmd.Description)
	}

	b.WriteString("\n## Quadlet Specifications\n\n")
	b.WriteString("### Supported Unit Kinds\n")
	for _, ext := range d.QuadletInfo.FileExtensions {
		fmt.Fprintf(&b, "- `%s`\n", ext)
	}

	b.WriteString("\n### Search Directory Precedence (First-match shadows lower)\n")
	for i, sp := range d.QuadletInfo.SearchPaths {
		fmt.Fprintf(&b, "%d. `%s`\n", i+1, sp)
	}

	b.WriteString("\n### Key Directives & Directives Cheatsheet\n")
	for _, dir := range d.QuadletInfo.Directives {
		fmt.Fprintf(&b, "- `%s`\n", dir)
	}

	b.WriteString("\n## Machine-Readable Tool Schema (JSON)\n\n```json\n")
	raw, _ := json.MarshalIndent(d.ToolSchema, "", "  ")
	b.Write(raw)
	b.WriteString("\n```\n")

	return b.String()
}

// JSON formats the definition as indented JSON.
func (d Definition) JSON() (string, error) {
	bytes, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
