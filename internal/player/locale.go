//go:build linux

package player

import (
	"ape-player/internal/diagnostics"
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
)

var localeOnce sync.Once
var localeErr error

// ConfigureNumericLocale must run on application startup after GTK initialization
// (which applies the user's LC_ALL), before creating any mpv instances or workers.
// libmpv requires LC_NUMERIC=C even for locales using a dot as decimal separator.
// Other categories, including UTF-8 text and translated messages, stay unchanged.
func ConfigureNumericLocale() error {
	localeOnce.Do(func() {
		handle, err := purego.Dlopen("libc.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			localeErr = fmt.Errorf("无法加载数字区域设置：%w", err)
			return
		}
		defer purego.Dlclose(handle)
		symbol, err := purego.Dlsym(handle, "setlocale")
		if err != nil {
			localeErr = fmt.Errorf("无法配置数字区域设置：%w", err)
			return
		}
		var setLocale func(int32, *byte) uintptr
		purego.RegisterFunc(&setLocale, symbol)
		const lcNumeric = 1 // Linux LC_NUMERIC from <locale.h>.
		before := cString(setLocale(lcNumeric, nil))
		value := []byte{'C', 0}
		result := setLocale(lcNumeric, &value[0])
		after := cString(result)
		diagnostics.Log("mpv.numeric_locale", "before", before, "after", after)
		if result == 0 || after != "C" {
			localeErr = fmt.Errorf("无法将播放器数字区域设置切换为 C（当前：%s）", after)
		}
	})
	return localeErr
}
