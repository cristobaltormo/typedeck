package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/cristobaltormo/typedeck/internal/app"
	"github.com/cristobaltormo/typedeck/internal/platform"
)

type Paths struct{ Dir string }

func (p Paths) Config() string { return filepath.Join(p.Dir, "config.json") }

func (p Paths) Backups() string { return filepath.Join(p.Dir, "backups") }

func (p Paths) Stats() string { return filepath.Join(p.Dir, "stats.json") }

func (p Paths) Keystrokes() string {
	return filepath.Join(platform.PrivateDir(p.Dir), "keystrokes.jsonl")
}

func DefaultPaths() Paths {
	p := Paths{Dir: platform.Current.ConfigDir()}
	parent := filepath.Dir(p.Dir)
	if _, err := os.Stat(p.Dir); errors.Is(err, os.ErrNotExist) {
		for _, old := range app.OldSlugs {
			dir := filepath.Join(parent, old)
			if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
				_ = copyDir(dir, p.Dir)
				break
			}
		}
	}
	platform.MigratePrivate(p.Dir, "keystrokes.jsonl")
	return p
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600)
	})
}

func Load(p Paths) (Config, error) {
	c, _, err := LoadWithNote(p)
	return c, err
}

func LoadWithNote(p Paths) (Config, string, error) {
	b, err := os.ReadFile(p.Config())
	if errors.Is(err, os.ErrNotExist) {
		c := Default()
		return c, "", Save(p, c, false, false)
	}
	if err != nil {
		return Config{}, "", err
	}
	c, perr := Parse(b)
	if perr == nil {
		return c, "", nil
	}
	broken := filepath.Join(p.Dir, "config.broken-"+time.Now().Format("20060102-150405")+".json")
	_ = os.Rename(p.Config(), broken)
	files, _ := filepath.Glob(filepath.Join(p.Backups(), "config-*.json"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if rc, err := Parse(raw); err == nil {
			if err := Save(p, rc, false, false); err == nil {
				return rc, fmt.Sprintf("the configuration was damaged (%v): backup %s was restored and the original was kept as %s", perr, filepath.Base(f), filepath.Base(broken)), nil
			}
		}
	}
	d := Default()
	return d, fmt.Sprintf("the configuration was damaged (%v) and there were no valid backups: starting from scratch; the original was kept as %s", perr, filepath.Base(broken)), Save(p, d, false, false)
}

func Save(p Paths, c Config, backup, force bool) error {
	c, err := Validate(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return err
	}
	if backup {
		if old, err := os.ReadFile(p.Config()); err == nil {
			_ = os.MkdirAll(p.Backups(), 0o755)
			files, _ := filepath.Glob(filepath.Join(p.Backups(), "config-*.json"))
			sort.Strings(files)
			recent := false
			if len(files) > 0 {
				if st, err := os.Stat(files[len(files)-1]); err == nil && time.Since(st.ModTime()) < 2*time.Minute {
					recent = true
				}
			}
			if force || !recent {
				name := filepath.Join(p.Backups(), "config-"+time.Now().Format("20060102-150405")+".json")
				_ = os.WriteFile(name, old, 0o600)
				files = append(files, name)
				sort.Strings(files)
				for len(files) > 40 {
					_ = os.Remove(files[0])
					files = files[1:]
				}
			}
		}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.Config() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.Config())
}

type BackupInfo struct {
	Name   string  `json:"name"`
	TS     float64 `json:"ts"`
	Layers int     `json:"layers"`
	Keys   int     `json:"keys"`
}

func ListBackups(p Paths) []BackupInfo {
	files, _ := filepath.Glob(filepath.Join(p.Backups(), "config-*.json"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	out := []BackupInfo{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		c, err := Parse(b)
		if err != nil {
			continue
		}
		st, _ := os.Stat(f)
		keys := len(c.Global)
		for _, l := range c.Layers {
			keys += len(l.Keys)
		}
		out = append(out, BackupInfo{Name: filepath.Base(f), TS: float64(st.ModTime().Unix()), Layers: len(c.Layers), Keys: keys})
	}
	return out
}

func RestoreBackup(p Paths, name string) (Config, error) {
	if !backupRe.MatchString(name) {
		return Config{}, errors.New("unknown backup")
	}
	b, err := os.ReadFile(filepath.Join(p.Backups(), name))
	if err != nil {
		return Config{}, err
	}
	c, err := Parse(b)
	if err != nil {
		return c, err
	}
	return c, Save(p, c, true, true)
}
