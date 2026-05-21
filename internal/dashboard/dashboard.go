// Package dashboard exposes the embedded Next.js static export as an http.FileSystem.
package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:web-out
var embedded embed.FS

// FS returns the embedded dashboard files as an http.FileSystem.
func FS() (http.FileSystem, error) {
	sub, err := fs.Sub(embedded, "web-out")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}
