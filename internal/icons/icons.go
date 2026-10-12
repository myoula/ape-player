// Package icons embeds the selected Lucide SVGs for native, offline rendering.
package icons

import (
	"embed"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

//go:embed lucide/*.svg
var files embed.FS

var library = func() map[string]*ui.SVG {
	entries, err := files.ReadDir("lucide")
	if err != nil {
		panic(err)
	}
	result := make(map[string]*ui.SVG, len(entries))
	for _, entry := range entries {
		data, err := files.ReadFile("lucide/" + entry.Name())
		if err != nil {
			panic(err)
		}
		result[strings.TrimSuffix(entry.Name(), ".svg")] = ui.MustParseSVG(data)
	}
	return result
}()

func Get(name string) *ui.SVG {
	if icon, ok := library[name]; ok {
		return icon
	}
	panic(fmt.Sprintf("unknown Lucide icon %q", name))
}
