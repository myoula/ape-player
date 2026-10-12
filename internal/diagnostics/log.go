// Package diagnostics records bounded, private playback logs without credentials.
package diagnostics

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var mu sync.Mutex
var secrets []string
var output *os.File
var urls = regexp.MustCompile(`https?://[^\s<>"']+`)
var credentials = regexp.MustCompile(`(?i)(authorization|x-emby-token|cookie|api[_-]?key)\s*[:=][^\r\n]*`)

func Path() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "ape-player", "playback.log")
}
func Init() (func(), error) {
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(Path()), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(Path(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return nil, err
	}
	output = f
	return func() {
		mu.Lock()
		defer mu.Unlock()
		if output != nil {
			output.Close()
			output = nil
		}
	}, nil
}

// Protect registers opaque credentials that may appear in native player messages.
func Protect(values ...string) {
	mu.Lock()
	defer mu.Unlock()
	for _, v := range values {
		if v != "" {
			secrets = append(secrets, v)
		}
	}
}
func URL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[invalid URL]"
	}
	u.User = nil
	u.Fragment = ""
	q := u.Query()
	for k := range q {
		q.Set(k, "REDACTED")
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func clean(s string) string {
	s = urls.ReplaceAllStringFunc(s, URL)
	s = credentials.ReplaceAllString(s, "$1: REDACTED")
	for _, secret := range secrets {
		s = strings.ReplaceAll(s, secret, "REDACTED")
	}
	return s
}
func Log(event string, fields ...any) {
	mu.Lock()
	defer mu.Unlock()
	record := map[string]any{"time": time.Now().Format(time.RFC3339Nano), "event": event}
	for i := 0; i+1 < len(fields); i += 2 {
		v := fields[i+1]
		switch value := v.(type) {
		case string:
			v = clean(value)
		case error:
			v = clean(value.Error())
		}
		record[fmt.Sprint(fields[i])] = v
	}
	b, err := json.Marshal(record)
	if err != nil {
		return
	}
	b = append(b, '\n')
	if output == nil {
		return
	}
	if stat, err := output.Stat(); err == nil && stat.Size()+int64(len(b)) > 4<<20 {
		output.Close()
		_ = os.Rename(Path(), Path()+".1")
		output, _ = os.OpenFile(Path(), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	}
	if output != nil {
		_, _ = output.Write(b)
	}
}
