package quadlet

import "strings"

// Deps returns the base names of other Quadlet units that u's file
// references: Pod=, Image=/Network=/Volume= pointing at quadlet files, and
// [Unit] dependency keys (Requires/Wants/After/Before/BindsTo/PartOf)
// naming generated units. Unknown or non-quadlet references are skipped.
func Deps(u Unit, f *File) []string {
	var out []string
	seen := map[string]bool{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || name == u.Name || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}

	sec := f.Section(string(u.Kind))
	if sec == nil {
		return nil
	}

	// Pod=foo.pod
	if pod := sec.Get("Pod"); pod != "" {
		add(stripQuadletExt(pod))
	}
	// Image=base.image / builder.build (plain image refs have no quadlet ext)
	if img := sec.Get("Image"); img != "" {
		if name := stripQuadletExt(img); name != img {
			add(name)
		}
	}
	// Network=net0.network or web.container (share netns of a container)
	for _, net := range sec.GetAll("Network") {
		if name := stripQuadletExt(net); name != net {
			add(name)
		}
	}
	// Volume=data.volume:/path — only quadlet references carry the ext
	for _, vol := range sec.GetAll("Volume") {
		src, _, _ := strings.Cut(vol, ":")
		if name := stripQuadletExt(src); name != src {
			add(name)
		}
	}

	// [Unit] keys reference generated unit names.
	unit := f.Section("Unit")
	for _, key := range []string{"Requires", "Wants", "After", "Before", "BindsTo", "PartOf"} {
		for _, val := range unit.GetAll(key) {
			for _, ref := range strings.Fields(val) {
				if name := unitToQuadletName(ref); name != "" {
					add(name)
				}
			}
		}
	}
	return out
}

var quadletExts = []string{".container", ".pod", ".kube", ".volume", ".network", ".image", ".build", ".artifact"}

// stripQuadletExt removes a known quadlet extension, returning the base
// name. It returns the input unchanged when no known extension matches.
func stripQuadletExt(s string) string {
	for _, ext := range quadletExts {
		if strings.HasSuffix(s, ext) {
			return strings.TrimSuffix(s, ext)
		}
	}
	return s
}

// unitToQuadletName maps a generated unit name back to its quadlet base
// name, or "" when the name does not look like a quadlet-generated unit.
func unitToQuadletName(unit string) string {
	if !strings.HasSuffix(unit, ".service") {
		return ""
	}
	base := strings.TrimSuffix(unit, ".service")
	for _, suffix := range []string{"-pod", "-volume", "-network", "-image", "-build", "-artifact"} {
		if strings.HasSuffix(base, suffix) {
			return strings.TrimSuffix(base, suffix)
		}
	}
	return base
}
