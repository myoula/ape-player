package diagnostics

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestPrivateRedactedLogs(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	closeLog, err := Init()
	if err != nil {
		t.Fatal(err)
	}
	defer closeLog()
	Protect("opaque-token", "opaque-header")
	Log("test", "url", "https://user:password@example.com/play?signature=signed-value&api_key=opaque-token#fragment", "message", "HTTP failure opaque-token opaque-header\nAuthorization: Bearer another-secret")
	b, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"user:password", "signed-value", "opaque-token", "opaque-header", "another-secret", "fragment"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("secret leaked: %s", secret)
		}
	}
	var record map[string]any
	if err = json.Unmarshal(b, &record); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(record["url"].(string), "example.com/play?") {
		t.Fatal("lost useful route")
	}
	stat, _ := os.Stat(Path())
	if stat.Mode().Perm() != 0600 {
		t.Fatal("log permissions")
	}
	// Crossing the cap keeps one bounded backup and starts a new log.
	if err = output.Truncate(4 << 20); err != nil {
		t.Fatal(err)
	}
	Log("rotated", "code", -13)
	if _, err = os.Stat(Path() + ".1"); err != nil {
		t.Fatal(err)
	}
	stat, _ = os.Stat(Path())
	if stat.Size() > 1024 || stat.Mode().Perm() != 0600 {
		t.Fatal("rotation failed")
	}
}
