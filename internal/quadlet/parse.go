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

// Parse reads and parses a Quadlet unit file. Lines outside any section,
// comments, and malformed lines are ignored, like systemd's parser does for
// well-formed generators. A trailing backslash continues the value on the
// next line, as in systemd's config syntax.
func Parse(path string) (*File, error) {
	fh, err := os.Open(path)
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

// Info is what quadman reads from one Quadlet source file.
type Info struct {
	// UnitName is the systemd unit the generator produces for the file,
	// with any ServiceName= override applied (the generator appends
	// ".service" to the override value).
	UnitName string
	// Image is the reference shown in the list: Image= for container and
	// image files, ImageTag= for build files.
	Image string
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
	return info
}
