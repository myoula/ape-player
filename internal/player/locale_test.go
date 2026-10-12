//go:build linux

package player

import (
	"os"
	"os/exec"
	"testing"

	"github.com/ebitengine/purego"
)

// Locale is process-global: isolate the native calls from the test runner and
// verify an environment override cannot undo the fix (LC_ALL beats LC_NUMERIC).
func TestNumericLocale(t *testing.T) {
	if os.Getenv("APE_LOCALE_TEST_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestNumericLocale$", "-test.v")
		cmd.Env = append(os.Environ(), "APE_LOCALE_TEST_CHILD=1", "LC_ALL=en_US.UTF-8", "LC_NUMERIC=de_DE.UTF-8")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("locale child: %s\n%v", out, err)
		} else {
			t.Log(string(out))
		}
		return
	}
	handle, err := purego.Dlopen("libc.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Fatal(err)
	}
	defer purego.Dlclose(handle)
	var setLocale func(int32, *byte) uintptr
	purego.RegisterLibFunc(&setLocale, handle, "setlocale")
	empty := byte(0)
	if setLocale(6, &empty) == 0 {
		t.Skip("en_US.UTF-8 locale is not installed")
	}
	before := cString(setLocale(1, nil))
	if before == "C" {
		t.Fatal("test did not enter a non-C numeric locale")
	}
	categories := []int32{0, 2, 3, 4, 5} // CTYPE, TIME, COLLATE, MONETARY, MESSAGES.
	original := map[int32]string{}
	for _, category := range categories {
		original[category] = cString(setLocale(category, nil))
	}
	for i := 0; i < 2; i++ {
		if err := ConfigureNumericLocale(); err != nil {
			t.Fatal(err)
		}
		if got := cString(setLocale(1, nil)); got != "C" {
			t.Fatalf("LC_NUMERIC=%s", got)
		}
	}
	for category, want := range original {
		if got := cString(setLocale(category, nil)); got != want {
			t.Fatalf("category %d: %q became %q", category, want, got)
		}
	}
	t.Logf("LC_NUMERIC %s -> C; all other categories preserved", before)
}
