package quadlet

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// File is a parsed INI-style Quadlet unit file.
type File struct {
	Path     string
	Sections map[string]*Section
}

// Section holds the keys of one INI section. Section and key names are
// case-sensitive, exactly like systemd's and Quadlet's own parsers: a file
// spelling "image=" is a key the generator rejects, so quadman must not
// read it as Image= either.
type Section struct {
	Name string
	keys map[string][]string
}

// Get returns the last value of key in the section, or "" when absent.
func (s *Section) Get(key string) string {
	vals := s.GetAll(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[len(vals)-1]
}

// GetAll returns every value of key, preserving file order. systemd allows
// repeating keys such as PublishPort= or Volume=.
func (s *Section) GetAll(key string) []string {
	if s == nil {
		return nil
	}
	return s.keys[key]
}

// Section returns the named section, or nil when the file has no such
// section.
func (f *File) Section(name string) *Section {
	if f == nil {
		return nil
	}
	return f.Sections[name]
}

// Section returns the name of the section a Quadlet file of this kind
// configures: [Container], [Pod], [Kube], …
func (k Kind) Section() string {
	switch k {
	case KindContainer:
		return "Container"
	case KindPod:
		return "Pod"
	case KindKube:
		return "Kube"
	case KindVolume:
		return "Volume"
	case KindNetwork:
		return "Network"
	case KindImage:
		return "Image"
	case KindBuild:
		return "Build"
	case KindArtifact:
		return "Artifact"
	}
	return ""
}

// Image returns the configured container image for [Container] files.
func (f *File) Image() string {
	return f.Section("Container").Get("Image")
}

// KubeYaml returns the Yaml= path declared in the [Kube] section, or ""
// when the file is not a .kube unit or declares none.
func (f *File) KubeYaml() string {
	return f.Section("Kube").Get("Yaml")
}

// SecretRef represents one secret reference from a Secret= directive in [Container].
type SecretRef struct {
	Name   string
	Type   string // "mount" (default) or "env"
	Target string
}

// ParseSecretRef decodes one Secret= value: "name[,type=env|mount][,target=…]".
func ParseSecretRef(raw string) (SecretRef, bool) {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	ref := SecretRef{Name: strings.TrimSpace(parts[0]), Type: "mount"}
	if ref.Name == "" {
		return SecretRef{}, false
	}
	for _, p := range parts[1:] {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "type":
			ref.Type = strings.TrimSpace(v)
		case "target":
			ref.Target = strings.TrimSpace(v)
		}
	}
	return ref, true
}

// Secrets returns all Secret= directives declared in the [Container] section.
func (f *File) Secrets() []SecretRef {
	var refs []SecretRef
	for _, raw := range f.Section("Container").GetAll("Secret") {
		if ref, ok := ParseSecretRef(raw); ok {
			refs = append(refs, ref)
		}
	}
	return refs
}

// Parse reads and parses a Quadlet unit file.
func Parse(path string) (*File, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- reading quadlet source files is this package's purpose
	if err != nil {
		return nil, err
	}
	return ParseBytes(path, data)
}

// ParseBytes parses Quadlet file content already in memory (e.g. read
// through a session runner from a remote host). Lines outside any section,
// comments, and malformed lines are ignored. A trailing backslash
// continues the value on the next line, as in systemd's config syntax.
func ParseBytes(path string, data []byte) (*File, error) {
	f := &File{Path: path, Sections: map[string]*Section{}}
	var cur *Section
	lines := strings.Split(string(data), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(strings.TrimRight(lines[i], "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		for strings.HasSuffix(line, "\\") && i+1 < len(lines) {
			i++
			line = strings.TrimRight(strings.TrimSuffix(line, "\\"), " \t") + " " + strings.TrimSpace(strings.TrimRight(lines[i], "\r"))
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSpace(line[1 : len(line)-1])
			if sec, exists := f.Sections[name]; exists {
				cur = sec
			} else {
				cur = &Section{Name: name, keys: map[string][]string{}}
				f.Sections[name] = cur
			}
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || cur == nil {
			continue
		}
		key = strings.TrimSpace(key)
		cur.keys[key] = append(cur.keys[key], strings.TrimSpace(val))
	}
	return f, nil
}

// bootKeys are the [Install] keys that make the generator wire a unit
// into a target at boot (or login).
var bootKeys = []string{"WantedBy", "RequiredBy", "UpheldBy"}

// BootTarget returns the first target the file's [Install] section wires
// the unit into (WantedBy=, then RequiredBy=, then UpheldBy=), or "" when
// the file does not start it at boot.
func (f *File) BootTarget() string {
	for _, k := range bootKeys {
		if v := f.Section("Install").Get(k); v != "" {
			return v
		}
	}
	return ""
}

// BootSource reports where u is enabled for boot: the unit file itself or
// one of its drop-ins (the last one in merge order that sets a boot key).
// It returns "" when neither does.
func BootSource(u Unit) string {
	src := ""
	if f, err := Parse(u.Path); err == nil && f.BootTarget() != "" {
		src = u.Path
	}
	for _, d := range Dropins(u) {
		if f, err := Parse(d.Path); err == nil && f.BootTarget() != "" {
			src = d.Path
		}
	}
	return src
}

// ErrBootInDropin is returned by RemoveBootTarget when the unit is enabled
// by a drop-in: editing only the main file would leave it enabled.
type ErrBootInDropin struct{ Path string }

func (e ErrBootInDropin) Error() string {
	return "enabled at boot by drop-in " + e.Path + ": edit or remove that file"
}

// EnsureBootTarget makes the file start the unit at boot with
// WantedBy=<target>. A file that already declares a boot key is left
// alone (the user controls boot behavior). An existing [Install] section
// gains the key; otherwise one is appended. It reports whether the file
// changed. Writing is atomic (see WriteFileAtomic).
func EnsureBootTarget(path, target string) (bool, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- editing the quadlet file the user pointed at is the feature
	if err != nil {
		return false, err
	}
	f, _ := ParseBytes(path, data)
	if f.BootTarget() != "" {
		return false, nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	installAt := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "[Install]" {
			installAt = i
		}
	}
	var body string
	if installAt >= 0 {
		out := append([]string{}, lines[:installAt+1]...)
		out = append(out, "WantedBy="+target)
		out = append(out, lines[installAt+1:]...)
		body = strings.Join(out, "\n") + "\n"
	} else {
		body = strings.Join(lines, "\n") + "\n\n[Install]\nWantedBy=" + target + "\n"
	}
	return true, WriteFileAtomic(path, []byte(body), 0o644)
}

// RemoveBootTarget stops the file from starting the unit at boot: it drops
// the boot keys (WantedBy=, RequiredBy=, UpheldBy=) from every [Install]
// section and removes sections that end up empty, keeping keys such as
// Alias= or DefaultInstance=. It reports whether the file changed.
func RemoveBootTarget(path string) (bool, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- editing the quadlet file the user pointed at is the feature
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(data), "\n")
	var out []string
	removed := false
	inInstall := false
	header := -1  // index in out of the current [Install] header
	kept := false // the current [Install] section kept a key
	continuing := false
	closeSection := func() {
		if header >= 0 && !kept {
			out = append(out[:header], out[header+1:]...)
		}
		header, kept = -1, false
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if continuing {
			continuing = strings.HasSuffix(trimmed, "\\")
			continue // a continued boot key line goes with its key
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if inInstall {
				closeSection()
			}
			inInstall = trimmed == "[Install]"
			if inInstall {
				header = len(out)
			}
			out = append(out, line)
			continue
		}
		if inInstall {
			key, _, ok := strings.Cut(trimmed, "=")
			if ok && isBootKey(strings.TrimSpace(key)) {
				removed = true
				continuing = strings.HasSuffix(trimmed, "\\")
				continue
			}
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, ";") {
				kept = true
			}
		}
		out = append(out, line)
	}
	if inInstall {
		closeSection()
	}
	if !removed {
		return false, nil
	}
	body := strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
	return true, WriteFileAtomic(path, []byte(body), 0o644)
}

func isBootKey(k string) bool {
	for _, b := range bootKeys {
		if k == b {
			return true
		}
	}
	return false
}

// WriteFileAtomic replaces path with data so readers only ever see the old
// or the new content, never a torn write:
//
//   - a symlink at path is resolved first, so the real file is replaced in
//     its own directory and the link (e.g. into a git checkout) survives;
//   - the data goes to a uniquely named temp file next to it (concurrent
//     writers never share one), is fsynced, then renamed over the target,
//     and the directory is fsynced so the rename survives a crash;
//   - the existing file's mode is kept (perm applies to new files), and so
//     is its owner when running as root (system mode editing a file
//     another account owns).
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	mode := perm
	uid, gid := -1, -1
	if fi, err := os.Stat(target); err == nil {
		mode = fi.Mode().Perm()
		if st, ok := fi.Sys().(*syscall.Stat_t); ok && os.Geteuid() == 0 {
			uid, gid = int(st.Uid), int(st.Gid)
		}
	}
	dir := filepath.Dir(target)
	// #nosec G703 -- the temp file sits next to the quadlet file the user asked quadman to edit
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup; a no-op once the rename below succeeds.
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if uid >= 0 {
		if err := tmp.Chown(uid, gid); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("keep owner of %s: %w", target, err)
		}
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil { // #nosec G304 -- the directory we just wrote into
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// writeAtomic is WriteFileAtomic for an existing quadlet file.
func writeAtomic(path string, data []byte) error {
	return WriteFileAtomic(path, data, 0o644)
}

// Info is what quadman reads from one Quadlet source file.
type Info struct {
	// UnitName is the systemd unit the generator produces for the file,
	// with any ServiceName= override applied (the generator appends
	// ".service" to the override value).
	UnitName string
	// Image is the reference shown in the list: Image= for container and
	// image files, ImageTag= for build files.
	Image string
	// AutoUpdate is the AutoUpdate= policy (registry/local), "" when unset.
	AutoUpdate string
	// ImageVolume is the ImageVolume= policy (Podman 6.1+), "" when unset.
	ImageVolume string
	// ContainerName is the ContainerName= override, "" for the default.
	ContainerName string
}

// InfoOf extracts u's Info from an already parsed file; a nil file yields
// the generator's default unit name only.
func InfoOf(u Unit, f *File) Info {
	info := Info{UnitName: UnitFileName(u.Name, u.Kind)}
	if f == nil {
		return info
	}
	sec := f.Section(u.Kind.Section())
	if sn := sec.Get("ServiceName"); sn != "" {
		info.UnitName = sn + ".service"
	}
	if u.Kind == KindBuild {
		info.Image = sec.Get("ImageTag")
	} else {
		info.Image = sec.Get("Image")
	}
	info.AutoUpdate = sec.Get("AutoUpdate")
	info.ImageVolume = sec.Get("ImageVolume")
	if u.Kind == KindContainer {
		info.ContainerName = sec.Get("ContainerName")
	}
	return info
}

// Inspect parses u's source file once and returns the effective unit name
// and image. Files that fail to parse keep the default generated name.
func Inspect(u Unit) Info {
	f, err := Parse(u.Path)
	if err != nil {
		return InfoOf(u, nil)
	}
	return InfoOf(u, f)
}

// ContainerName returns the name podman gives u's container: the
// ContainerName= override with the systemd specifiers Quadlet passes
// through (%N, %n, %p, %i, %%) expanded, or the generator's default
// systemd-%N. unitName is the generated unit (ServiceName= applied).
func ContainerName(unitName, override string) string {
	if override == "" {
		override = "systemd-%N"
	}
	full := unitName
	base := strings.TrimSuffix(unitName, ".service")
	prefix, instance := base, ""
	if at := strings.IndexByte(base, '@'); at >= 0 {
		prefix, instance = base[:at], base[at+1:]
	}
	var b bytes.Buffer
	for i := 0; i < len(override); i++ {
		c := override[i]
		if c != '%' || i+1 >= len(override) {
			b.WriteByte(c)
			continue
		}
		i++
		switch override[i] {
		case 'N':
			b.WriteString(base)
		case 'n':
			b.WriteString(full)
		case 'p':
			b.WriteString(prefix)
		case 'i':
			b.WriteString(instance)
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(override[i])
		}
	}
	return b.String()
}
