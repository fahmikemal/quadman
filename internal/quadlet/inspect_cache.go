package quadlet

import (
	"os"
	"sync"
)

// inspectCacheEntry is one cached Inspect result with the source mtime it
// was parsed from.
type inspectCacheEntry struct {
	mtime int64
	info  Info
}

// InspectCache memoizes Inspect by source file mtime, so the 2.5s UI poll
// only re-parses files that actually changed on disk. The zero value is
// ready to use. It is safe for concurrent use: the UI runs refreshCmd on a
// background goroutine while actions parse files on the main loop.
type InspectCache struct {
	mu      sync.Mutex
	entries map[string]inspectCacheEntry
}

// Inspect returns the cached Info for u when its file is unchanged, and
// re-parses otherwise. Files that fail to stat or parse fall back to a fresh
// Inspect call, matching the uncached behavior.
func (c *InspectCache) Inspect(u Unit) Info {
	mtime := fileMtime(u.Path)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries != nil {
		if e, ok := c.entries[u.Path]; ok && e.mtime == mtime {
			return e.info
		}
	}
	info := Inspect(u)
	if c.entries == nil {
		c.entries = map[string]inspectCacheEntry{}
	}
	c.entries[u.Path] = inspectCacheEntry{mtime: mtime, info: info}
	return info
}

// fileMtime returns the file's modification time as Unix nanos, or 0 when
// the file cannot be statted.
func fileMtime(path string) int64 {
	if fi, err := os.Stat(path); err == nil {
		return fi.ModTime().UnixNano()
	}
	return 0
}

// InspectAll resolves every unit's Info through the cache and reports the
// infos in unit order along with the parallel image list the UI table needs.
func (c *InspectCache) InspectAll(units []Unit) ([]Info, []string) {
	infos := make([]Info, len(units))
	images := make([]string, len(units))
	for i := range units {
		infos[i] = c.Inspect(units[i])
		units[i].UnitName = infos[i].UnitName
		images[i] = infos[i].Image
	}
	return infos, images
}
