package quadlet

import (
	"bufio"
	"os"
	"strings"
)

// File is a parsed INI-style Quadlet unit file.
type File struct {
	Path     string
	Sections map[string]*Section
}

// Section holds the keys of one INI section. Section and key lookups are
// case-insensitive, matching systemd's behavior.
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
	return s.keys[strings.ToLower(key)]
}

// Section returns the named section, or nil when the file has no such
// section. Lookup is case-insensitive.
func (f *File) Section(name string) *Section {
	if f == nil {
		return nil
	}
	return f.Sections[strings.ToLower(name)]
}

// Image returns the configured container image for [Container] files.
func (f *File) Image() string {
	return f.Section("Container").Get("Image")
}

// SecretRef represents one secret reference from a Secret= directive in [Container].
type SecretRef struct {
	Name   string
	Type   string // "mount" (default) or "env"
	Target string
}

// Secrets returns all Secret= directives declared in the [Container] section.
func (f *File) Secrets() []SecretRef {
	sec := f.Section("Container")
	if sec == nil {
		return nil
	}
	raw := sec.GetAll("Secret")
	if len(raw) == 0 {
		return nil
	}
	var refs []SecretRef
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		parts := strings.Split(r, ",")
		ref := SecretRef{
			Name: strings.TrimSpace(parts[0]),
			Type: "mount",
		}
		for _, p := range parts[1:] {
			k, v, ok := strings.Cut(p, "=")
			if !ok {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "type":
				ref.Type = strings.TrimSpace(v)
			case "target":
				ref.Target = strings.TrimSpace(v)
			}
		}
		refs = append(refs, ref)
	}
	return refs
}

// Parse reads and parses a Quadlet unit file. Lines outside any section,
// comments, and malformed lines are ignored, like systemd's parser does for
// well-formed generators. A trailing backslash continues the value on the
// next line, as in systemd's config syntax.
func Parse(path string) (*File, error) {
	fh, err := os.Open(path) // #nosec G304 -- reading quadlet source files is this package's purpose
	if err != nil {
		return nil, err
	}
	defer fh.Close()

	f := &File{Path: path, Sections: map[string]*Section{}}
	var cur *Section
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		for strings.HasSuffix(line, "\\") && sc.Scan() {
			line = strings.TrimRight(strings.TrimSuffix(line, "\\"), " \t") + " " + strings.TrimSpace(sc.Text())
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSpace(line[1 : len(line)-1])
			lower := strings.ToLower(name)
			if sec, exists := f.Sections[lower]; exists {
				cur = sec
			} else {
				cur = &Section{Name: name, keys: map[string][]string{}}
				f.Sections[lower] = cur
			}
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || cur == nil {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		cur.keys[key] = append(cur.keys[key], strings.TrimSpace(val))
	}
	return f, sc.Err()
}

// BootTarget returns the [Install] WantedBy= target declared in the file,
// which is how quadlet units start at boot (the generator turns it into a
// wants symlink on the next daemon-reload). "" when not enabled for boot.
func (f *File) BootTarget() string {
	return f.Section("Install").Get("WantedBy")
}

// EnsureBootTarget appends an [Install] section with WantedBy=<target> when
// the file does not declare one. It reports whether the file changed.
// Writing is atomic (temp file + rename) so a crash cannot corrupt the file.
func EnsureBootTarget(path, target string) (bool, error) {
	f, err := Parse(path)
	if err != nil {
		return false, err
	}
	if f.BootTarget() != "" {
		return false, nil // user already controls boot behavior; don't touch
	}
	data, err := os.ReadFile(path) // #nosec G304 -- editing the quadlet file the user pointed at is the feature
	if err != nil {
		return false, err
	}
	body := strings.TrimRight(string(data), "\n") + "\n\n[Install]\nWantedBy=" + target + "\n"
	return true, writeAtomic(path, []byte(body))
}

// RemoveBootTarget deletes the [Install] section from the file, disabling
// boot start. It reports whether the file changed.
func RemoveBootTarget(path string) (bool, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- editing the quadlet file the user pointed at is the feature
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(data), "\n")
	var out []string
	inInstall := false
	removed := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isHeader := strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")
		if isHeader {
			inInstall = strings.EqualFold(strings.Trim(trimmed, "[]"), "Install")
			if inInstall {
				removed = true
				continue
			}
		}
		if inInstall {
			continue // drop [Install] section lines
		}
		out = append(out, line)
	}
	if !removed {
		return false, nil
	}
	// Collapse trailing blank lines left by the removal.
	body := strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
	return true, writeAtomic(path, []byte(body))
}

// writeAtomic writes data to a temp file next to path, then renames it over
// path, preserving the original file mode.
func writeAtomic(path string, data []byte) error {
	var mode os.FileMode = 0o644
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode()
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil { // #nosec G703 -- path is the quadlet file the user asked quadman to edit; the temp file sits next to it
		return err
	}
	return os.Rename(tmp, path)
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
}

// Inspect parses u's source file once and returns the effective unit name
// and image. Files that fail to parse keep the default generated name.
func Inspect(u Unit) Info {
	info := Info{UnitName: UnitFileName(u.Name, u.Kind)}
	f, err := Parse(u.Path)
	if err != nil {
		return info
	}
	sec := f.Section(string(u.Kind))
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
	return info
}
