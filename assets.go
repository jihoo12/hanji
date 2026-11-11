// Package hanji embeds the built-in theme and the starter project.
package hanji

import (
	"embed"
	"io/fs"
)

//go:embed all:theme all:starter
var files embed.FS

// Theme holds layouts/, components/ and assets/ of the built-in theme.
func Theme() fs.FS {
	sub, _ := fs.Sub(files, "theme")
	return sub
}

// Starter is the project `hanji new` writes out.
func Starter() fs.FS {
	sub, _ := fs.Sub(files, "starter")
	return sub
}
