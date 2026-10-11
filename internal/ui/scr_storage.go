package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
)

// Storage screen (g): `podman system df` as tiles, then images, volumes
// and containers as sub-tabs. Every object links back to the quadlet
// units that use it (USED BY), and objects nothing uses group under
// UNUSED with the space a prune would reclaim.

type storageData struct {
	df       []podman.DfRow
	images   []podman.ImageEntry
	volumes  []podman.VolumeEntry
	volSizes map[string]string
	ctrs     []podman.ContainerEntry
	// volumeUsers maps a volume name to the quadlet units referencing it
	// (Volume= in .container files, or the .volume unit that defines it).
	volumeUsers map[string][]string
	loadedAt    time.Time
	partial     []string // fetches that failed; the rest still renders
}

type storageMsg struct {
	data storageData
	err  error
}

func (s storageMsg) screenMode() mode        { return modeStorage }
func (s storageMsg) screenErrPrefix() string { return "system df" }
func (s storageMsg) screenErr() error        { return s.err }
func (s storageMsg) apply(m *Model)          { m.storage = s.data }

// storageCmd loads the inventory. Only the df summary is required; any
// other fetch that fails is reported in the status line instead.
func (m Model) storageCmd() tea.Cmd {
	units := slices.Clone(m.units)
	parse := m.parseUnitFile
	return func() tea.Msg {
		ctx := context.Background()
		var d storageData
		var err error
		if d.df, err = podman.SystemDfSummary(ctx); err != nil {
			return storageMsg{err: err}
		}
		if d.images, err = podman.Images(ctx); err != nil {
			d.partial = append(d.partial, "images")
		}
		if d.volumes, err = podman.Volumes(ctx); err != nil {
			d.partial = append(d.partial, "volumes")
		}
		if d.ctrs, err = podman.Containers(ctx, true); err != nil {
			d.partial = append(d.partial, "containers")
		}
		if d.volSizes, err = podman.VolumeSizes(ctx); err != nil {
			d.partial = append(d.partial, "volume sizes")
		}
		d.volumeUsers = volumeReferences(units, parse)
		d.loadedAt = time.Now()
		return storageMsg{data: d}
	}
}

// volumeReferences resolves which quadlet units use which named volume:
// a .volume unit defines VolumeName= (default systemd-<name>); a
// .container's Volume=src:dst names it either as <x>.volume or as a bare
// named volume (anything that is not a path).
func volumeReferences(units []quadlet.Unit, parse func(quadlet.Unit) (*quadlet.File, error)) map[string][]string {
	volName := map[string]string{} // .volume base name -> podman volume name
	files := map[string]*quadlet.File{}
	for _, u := range units {
		f, err := parse(u)
		if err != nil {
			continue
		}
		files[u.Name] = f
		if u.Kind == quadlet.KindVolume {
			name := f.Section("Volume").Get("VolumeName")
			if name == "" {
				name = "systemd-" + u.Name
			}
			volName[u.Name] = name
		}
	}
	out := map[string][]string{}
	add := func(vol, unit string) {
		if !slices.Contains(out[vol], unit) {
			out[vol] = append(out[vol], unit)
		}
	}
	for _, u := range units {
		f := files[u.Name]
		if f == nil {
			continue
		}
		if u.Kind == quadlet.KindVolume {
			add(volName[u.Name], u.UnitName)
			continue
		}
		for _, v := range f.Section(string(u.Kind)).GetAll("Volume") {
			src, _, _ := strings.Cut(v, ":")
			switch {
			case strings.HasSuffix(src, ".volume"):
				base := strings.TrimSuffix(src, ".volume")
				if n, ok := volName[base]; ok {
					add(n, u.UnitName)
				} else {
					add("systemd-"+base, u.UnitName)
				}
			case src != "" && !strings.ContainsAny(src[:1], "/.~%"):
				add(src, u.UnitName)
			}
		}
	}
	return out
}

// ctrUnit is the quadlet unit behind a container: the PODMAN_SYSTEMD_UNIT
// label when it names one of our quadlet units (podman-compose and
// `podman generate systemd` stamp the same label on their own units), or
// the default systemd-<name> container name.
func (m Model) ctrUnit(c podman.ContainerEntry) string {
	if u := c.SystemdUnit(); u != "" {
		if _, ok := m.unitByName(u); ok {
			return u
		}
		return ""
	}
	if name, ok := strings.CutPrefix(c.Name(), "systemd-"); ok {
		if i, ok := m.unitByName(name); ok {
			return m.units[i].UnitName
		}
	}
	return ""
}

// imageUsers lists the units whose containers run img, plus units whose
// quadlet file names one of its tags.
func (m Model) imageUsers(img podman.ImageEntry) []string {
	var users []string
	add := func(u string) {
		if u != "" && !slices.Contains(users, u) {
			users = append(users, u)
		}
	}
	for _, c := range m.storage.ctrs {
		if c.ImageID == img.ID {
			add(m.ctrUnit(c))
		}
	}
	for _, u := range m.units {
		if ref := m.images[u.UnitName]; ref != "" && imageRefMatches(img.Names, ref) {
			add(u.UnitName)
		}
	}
	return users
}

// imageRefMatches compares a quadlet Image= value with an image's fully
// qualified names: exact, or the name ends with "/"+ref (short names), with
// ":latest" implied when the ref carries no tag or digest.
func imageRefMatches(names []string, ref string) bool {
	if ref == "" {
		return false
	}
	last := ref[strings.LastIndex(ref, "/")+1:]
	if !strings.ContainsAny(last, ":@") {
		ref += ":latest"
	}
	for _, n := range names {
		if n == ref || strings.HasSuffix(n, "/"+ref) {
			return true
		}
	}
	return false
}

func (m Model) usersCell(users []string, fallback string) scrCell {
	if len(users) == 0 {
		return cell(glyph(gWarn)+" "+fallback, warnStyle)
	}
	parts := make([]string, 0, len(users))
	for _, u := range users {
		parts = append(parts, m.unitGlyph(u))
	}
	return cell(strings.Join(parts, " · "), fgStyle)
}

func (m Model) storageSpec() screenSpec {
	d := m.storage
	var imgN, volN, ctrN int
	var imgSz, volSz, ctrSz, reclaim int64
	for _, r := range d.df {
		reclaim += r.RawReclaimable
		switch r.Type {
		case "Images":
			imgN, imgSz = r.Total, r.RawSize
		case "Local Volumes":
			volN, volSz = r.Total, r.RawSize
		case "Containers":
			ctrN, ctrSz = r.Total, r.RawSize
		}
	}
	// df counts intermediate layers as images; show what the tabs list.
	if d.images != nil {
		imgN = len(d.images)
	}
	if d.volumes != nil {
		volN = len(d.volumes)
	}
	if d.ctrs != nil {
		ctrN = len(d.ctrs)
	}
	tiles := []tile{
		{label: "IMAGES", value: fmt.Sprintf("%d", imgN), note: podman.HumanBytes(imgSz), valueStyle: brightStyle},
		{label: "VOLUMES", value: fmt.Sprintf("%d", volN), note: podman.HumanBytes(volSz), valueStyle: brightStyle},
		{label: "CONTAINERS", value: fmt.Sprintf("%d", ctrN), note: podman.HumanBytes(ctrSz), valueStyle: brightStyle},
		{label: "RECLAIMABLE", value: podman.HumanBytes(reclaim), note: "P prune · asks first", valueStyle: warnStyle},
	}
	if reclaim > 0 {
		tiles[3].tint = activePalette.warn
	} else {
		tiles[3].valueStyle = okStyle
	}
	s := screenSpec{
		tiles: tiles,
		tabs: []string{
			fmt.Sprintf("Images %d", len(d.images)),
			fmt.Sprintf("Volumes %d", len(d.volumes)),
			fmt.Sprintf("Containers %d", len(d.ctrs)),
		},
		toolbar: dimStyle.Render(" sort: ") + fgStyle.Render(m.storageSortName()) + "  " + keyHint("S", "cycle"),
	}
	var unusedImg, unusedVol int64
	switch m.scrTab {
	case 1:
		s.cols, s.rows, unusedVol = m.volumeRows()
		s.empty = "no named volumes"
		s.detail = m.volumeDetail
	case 2:
		s.cols, s.rows = m.containerRows()
		s.empty = "no containers"
		s.detail = m.containerDetail
	default:
		s.cols, s.rows, unusedImg = m.imageRows()
		s.empty = "no images"
		s.detail = m.imageDetail
	}
	s.info = fmt.Sprintf("podman system df · %s reclaimable", podman.HumanBytes(reclaim))
	if unusedImg > 0 {
		s.info += " · unused images " + podman.HumanBytes(unusedImg)
	}
	if unusedVol > 0 {
		s.info += " · unused volumes " + podman.HumanBytes(unusedVol)
	}
	if len(d.partial) > 0 {
		s.info += " · could not load: " + strings.Join(d.partial, ", ")
	}
	return s
}

func (m Model) storageSortName() string {
	if m.scrSort == 1 {
		return "name"
	}
	return "size ▾"
}

func (m Model) imageRows() ([]scrCol, []scrRow, int64) {
	cols := []scrCol{
		{title: "REPOSITORY:TAG", min: 20},
		{title: "IMAGE ID", w: 12},
		{title: "SIZE", w: 8, right: true},
		{title: "CREATED", w: 13},
		{title: "USED BY", min: 14, max: 40, hide: breakpointMid},
	}
	imgs := slices.Clone(m.storage.images)
	sortStorage(m, imgs, func(e podman.ImageEntry) (string, int64) { return e.Ref(), e.Size })
	var used, unused []scrRow
	var unusedSize int64
	for _, img := range imgs {
		users := m.imageUsers(img)
		r := scrRow{
			id: "img:" + img.ID,
			cells: []scrCell{
				cell(img.Ref(), fgStyle),
				cell(shortHex(img.ID), mutedStyle),
				cell(podman.HumanBytes(img.Size), fgStyle),
				cell(ago(time.Unix(img.Created, 0)), mutedStyle),
			},
		}
		if len(users) > 0 {
			r.unit = users[0]
		}
		if len(users) == 0 && img.Containers == 0 {
			r.cells = append(r.cells, m.usersCell(nil, "unused"))
			if img.Dangling() {
				r.cells[0].style = mutedStyle
			}
			unused = append(unused, r)
			unusedSize += img.Size
			continue
		}
		if len(users) == 0 {
			r.cells = append(r.cells, cell(fmt.Sprintf("%d container(s), not quadlet", img.Containers), mutedStyle))
		} else {
			r.cells = append(r.cells, m.usersCell(users, ""))
		}
		used = append(used, r)
	}
	rows := used
	if len(unused) > 0 {
		rows = append(rows, scrRow{heading: fmt.Sprintf("UNUSED · not used by any container or unit · %d · %s", len(unused), podman.HumanBytes(unusedSize))})
		rows = append(rows, unused...)
	}
	return cols, rows, unusedSize
}

func (m Model) volumeRows() ([]scrCol, []scrRow, int64) {
	cols := []scrCol{
		{title: "VOLUME", min: 18, max: 44},
		{title: "DRIVER", w: 7},
		{title: "SIZE", w: 8, right: true},
		{title: "CREATED", w: 13},
		{title: "USED BY", min: 14, hide: breakpointMid},
	}
	vols := slices.Clone(m.storage.volumes)
	sortStorage(m, vols, func(v podman.VolumeEntry) (string, int64) {
		return v.Name, podman.ParseBytes(m.storage.volSizes[v.Name])
	})
	var used, unused []scrRow
	var unusedSize int64
	for _, v := range vols {
		users := m.storage.volumeUsers[v.Name]
		size := m.storage.volSizes[v.Name]
		if size == "" {
			size = "—"
		}
		r := scrRow{
			id: "vol:" + v.Name,
			cells: []scrCell{
				cell(v.Name, fgStyle),
				cell(v.Driver, mutedStyle),
				cell(size, fgStyle),
				cell(agoRFC3339(v.CreatedAt), mutedStyle),
			},
		}
		switch {
		case len(users) > 0:
			r.unit = users[0]
			r.cells = append(r.cells, m.usersCell(users, ""))
			used = append(used, r)
		case v.MountCount > 0:
			r.cells = append(r.cells, cell(fmt.Sprintf("mounted %d×, not quadlet", v.MountCount), mutedStyle))
			used = append(used, r)
		default:
			r.cells = append(r.cells, m.usersCell(nil, "unused"))
			unused = append(unused, r)
			unusedSize += podman.ParseBytes(m.storage.volSizes[v.Name])
		}
	}
	rows := used
	if len(unused) > 0 {
		rows = append(rows, scrRow{heading: fmt.Sprintf("UNUSED · no unit references them · %d · %s", len(unused), podman.HumanBytes(unusedSize))})
		rows = append(rows, unused...)
	}
	return cols, rows, unusedSize
}

func (m Model) containerRows() ([]scrCol, []scrRow) {
	cols := []scrCol{
		{title: "NAME", min: 16, max: 36},
		{title: "STATE", w: 10},
		{title: "IMAGE", min: 14},
		{title: "SIZE", w: 8, right: true},
		{title: "CREATED", w: 13},
		{title: "UNIT", min: 12, max: 30, hide: breakpointMid},
	}
	ctrs := slices.Clone(m.storage.ctrs)
	sortStorage(m, ctrs, func(c podman.ContainerEntry) (string, int64) {
		if c.Size == nil {
			return c.Name(), 0
		}
		return c.Name(), c.Size.RW
	})
	var quad, other []scrRow
	for _, c := range ctrs {
		size := "—"
		if c.Size != nil {
			size = podman.HumanBytes(c.Size.RW)
		}
		g := gInactive
		if c.State == "running" {
			g = gActive
		}
		unit := m.ctrUnit(c)
		r := scrRow{
			id:   "ctr:" + c.ID,
			unit: unit,
			cells: []scrCell{
				cell(c.Name(), fgStyle),
				cell(glyph(g)+" "+c.State, glyphStyle(g)),
				cell(c.Image, mutedStyle),
				cell(size, fgStyle),
				cell(ago(time.Unix(c.Created, 0)), mutedStyle),
			},
		}
		if unit != "" {
			r.cells = append(r.cells, cell(m.unitGlyph(unit), fgStyle))
			quad = append(quad, r)
		} else {
			r.cells = append(r.cells, cell("—", dimStyle))
			other = append(other, r)
		}
	}
	rows := quad
	if len(other) > 0 {
		rows = append(rows, scrRow{heading: fmt.Sprintf("OTHER · not created by quadlet · %d", len(other))})
		rows = append(rows, other...)
	}
	return cols, rows
}

// sortStorage orders items by size (largest first) or by name.
func sortStorage[T any](m Model, items []T, key func(T) (string, int64)) {
	slices.SortStableFunc(items, func(a, b T) int {
		an, as := key(a)
		bn, bs := key(b)
		if m.scrSort == 0 && as != bs {
			if as > bs {
				return -1
			}
			return 1
		}
		return strings.Compare(an, bn)
	})
}

func (m Model) imageByID(id string) (podman.ImageEntry, bool) {
	for _, img := range m.storage.images {
		if img.ID == id {
			return img, true
		}
	}
	return podman.ImageEntry{}, false
}

func (m Model) imageDetail(r scrRow, _ int) []string {
	img, ok := m.imageByID(strings.TrimPrefix(r.id, "img:"))
	if !ok {
		return nil
	}
	users := m.imageUsers(img)
	lines := []string{
		brightStyle.Render(img.Ref()),
		detailField("ID", shortHex(img.ID)+" · "+podman.HumanBytes(img.Size)+" · created "+time.Unix(img.Created, 0).Format("2006-01-02 15:04")),
	}
	if len(img.Names) > 1 {
		lines = append(lines, detailField("Also tagged", strings.Join(img.Names[1:], ", ")))
	}
	switch {
	case len(users) > 0:
		lines = append(lines, detailField("Used by", m.usersCell(users, "").text))
	case img.Containers > 0:
		lines = append(lines, detailField("Used by", fmt.Sprintf("%d container(s) not created by quadlet", img.Containers)))
	default:
		lines = append(lines, detailField("Used by", warnStyle.Render("nothing — a prune would remove it")))
	}
	lines = append(lines, keyHint("p", "pull newer")+"   "+keyHint("D", "remove")+"   "+keyHint("y", "copy ref")+"   "+keyHint("↵", "open unit"))
	return lines
}

func (m Model) volumeDetail(r scrRow, _ int) []string {
	name := strings.TrimPrefix(r.id, "vol:")
	for _, v := range m.storage.volumes {
		if v.Name != name {
			continue
		}
		lines := []string{
			brightStyle.Render(v.Name),
			detailField("Driver", v.Driver+" · created "+agoRFC3339(v.CreatedAt)+" · size "+orDash(m.storage.volSizes[v.Name])),
		}
		if users := m.storage.volumeUsers[v.Name]; len(users) > 0 {
			lines = append(lines, detailField("Used by", m.usersCell(users, "").text))
		} else if v.MountCount == 0 {
			lines = append(lines, detailField("Used by", warnStyle.Render("no unit references it — data stays until you remove it")))
		}
		lines = append(lines, keyHint("D", "remove")+"   "+keyHint("y", "copy name")+"   "+keyHint("↵", "open unit"))
		return lines
	}
	return nil
}

func (m Model) containerDetail(r scrRow, _ int) []string {
	id := strings.TrimPrefix(r.id, "ctr:")
	for _, c := range m.storage.ctrs {
		if c.ID != id {
			continue
		}
		lines := []string{
			brightStyle.Render(c.Name()),
			detailField("Status", c.Status+" · "+shortHex(c.ID)),
			detailField("Image", c.Image),
		}
		if u := m.ctrUnit(c); u != "" {
			lines = append(lines, detailField("Unit", m.unitGlyph(u)+"  (quadlet runs it with --rm: stopping the unit removes it)"))
		}
		lines = append(lines, keyHint("D", "remove stopped")+"   "+keyHint("y", "copy name")+"   "+keyHint("↵", "open unit"))
		return lines
	}
	return nil
}

// storageKeysIn handles the storage screen's own keys.
func (m Model) storageKeysIn(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "r":
		return m, tea.Batch(m.setBusy("system df"), m.storageCmd()), true
	case "S":
		m.scrSort = (m.scrSort + 1) % 2
		return m, nil, true
	case "P":
		return m.pruneKeys(msg)
	case "y":
		r, ok := m.selectedRow()
		if !ok {
			return m, nil, true
		}
		text := r.cells[0].text
		m.setStatus("copied "+text, false)
		return m, tea.SetClipboard(text), true
	case "p":
		r, ok := m.selectedRow()
		if !ok || m.scrTab != 0 {
			return m, nil, true
		}
		if m.refuseReadonly() {
			return m, nil, true
		}
		img, ok := m.imageByID(strings.TrimPrefix(r.id, "img:"))
		if !ok || img.Dangling() {
			m.setStatus("untagged image: nothing to pull", false)
			return m, nil, true
		}
		ref := img.Ref()
		return m, tea.Batch(m.setBusy("pull "+ref), actionCmdHint("pull "+ref, "restart its units to run the new image",
			func(ctx context.Context) (string, error) { return podman.PullImage(ctx, ref) })), true
	case "D":
		return m.storageRemove()
	}
	return m, nil, false
}

// storageRemove arms removal of the selected image, volume or container.
// Anything a quadlet unit still uses is refused with the reason.
func (m Model) storageRemove() (tea.Model, tea.Cmd, bool) {
	if m.refuseReadonly() {
		return m, nil, true
	}
	r, ok := m.selectedRow()
	if !ok {
		return m, nil, true
	}
	kind, id, _ := strings.Cut(r.id, ":")
	var users []string
	var label string
	var run func(context.Context) error
	switch kind {
	case "img":
		img, _ := m.imageByID(id)
		users, label = m.imageUsers(img), "image "+img.Ref()
		run = func(ctx context.Context) error { return podman.ImageRemove(ctx, id) }
	case "vol":
		users, label = m.storage.volumeUsers[id], "volume "+id
		run = func(ctx context.Context) error { return podman.VolumeRemove(ctx, id) }
	case "ctr":
		for _, c := range m.storage.ctrs {
			if c.ID == id {
				label = "container " + c.Name()
				if c.State == "running" {
					m.setStatus(c.Name()+" is running — stop its unit first", true)
					return m, nil, true
				}
			}
		}
		run = func(ctx context.Context) error { return podman.ContainerRemove(ctx, id) }
	}
	if len(users) > 0 {
		m.setStatus(label+" is used by "+strings.Join(users, ", ")+" — remove or change those units first", true)
		return m, nil, true
	}
	lines := []string{"remove " + label}
	keeps := []string{"every quadlet file", "other images, volumes and containers"}
	if kind == "vol" {
		lines = append(lines, "delete the data stored in it — this cannot be undone")
	}
	m.pending = &pendingAction{
		verb:   "remove",
		title:  "Remove " + label + "?",
		lines:  lines,
		keeps:  keeps,
		danger: kind == "vol",
		run: func(m Model) (tea.Model, tea.Cmd) {
			return m, tea.Batch(m.setBusy("remove "+label), actionCmdHint("remove "+label, "", func(ctx context.Context) (string, error) {
				return "", run(ctx)
			}), m.storageCmd())
		},
	}
	return m, nil, true
}

// shortHex is the 12-character ID podman prints.
func shortHex(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// ago renders a past time as "4s ago", "19h ago", "12 days ago".
func ago(t time.Time) string {
	if t.IsZero() || t.Unix() <= 0 {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < 0:
		return "just now"
	case d < 48*time.Hour:
		return humanDuration(d) + " ago"
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%d weeks ago", int(d.Hours()/24/7))
	case d < 730*24*time.Hour:
		return fmt.Sprintf("%d months ago", int(d.Hours()/24/30))
	}
	return fmt.Sprintf("%d years ago", int(d.Hours()/24/365))
}

func agoRFC3339(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return "—"
	}
	return ago(t)
}
