package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Source struct {
	ID       string
	Name     string
	URL      string
	Username string
	UserID   string
	Token    string
	Remember bool
	Timeout  int
	Bitrate  int
}
type Settings struct {
	AudioLanguage    string
	SubtitleLanguage string
	AutoNext         bool
	Volume           float64
}
type Config struct {
	Sources      []Source
	Active       string
	DeviceID     string
	Settings     Settings
	HiddenResume map[string]map[string]time.Time `json:",omitempty"`
}

func ID() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func Default() Config {
	return Config{DeviceID: ID(), Settings: Settings{AudioLanguage: "jpn,eng", SubtitleLanguage: "zho,chi,eng", AutoNext: true, Volume: 80}}
}
func Path() string {
	d, e := os.UserConfigDir()
	if e != nil {
		d = "."
	}
	return filepath.Join(d, "ape-player", "config.json")
}
func Load(path string) (Config, error) {
	c := Default()
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	if c.DeviceID == "" {
		c.DeviceID = ID()
	}
	return c, e
}
func Save(path string, c Config) error {
	c.Sources = append([]Source(nil), c.Sources...)
	for i := range c.Sources {
		if !c.Sources[i].Remember {
			c.Sources[i].Token = ""
			c.Sources[i].UserID = ""
		}
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".config-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), path)
}
