package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRememberAndPermissions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ape", "config.json")
	c := Default()
	c.Sources = []Source{{ID: "a", Token: "secret", UserID: "u", Remember: false}, {ID: "b", Token: "saved", Remember: true}}
	if e := Save(p, c); e != nil {
		t.Fatal(e)
	}
	if c.Sources[0].Token != "secret" {
		t.Fatal("save mutated live login")
	}
	got, e := Load(p)
	if e != nil {
		t.Fatal(e)
	}
	if got.Sources[0].Token != "" || got.Sources[1].Token != "saved" {
		t.Fatal("remember flag ignored")
	}
	info, _ := os.Stat(p)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if got.DeviceID != c.DeviceID {
		t.Fatal("device identity changed")
	}
}
func TestCorruptConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	_ = os.WriteFile(p, []byte("broken"), 0600)
	if _, e := Load(p); e == nil {
		t.Fatal("invalid config ignored")
	}
}
