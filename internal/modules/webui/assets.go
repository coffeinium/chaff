package webui

import (
	"embed"
	"io/fs"
)

//go:embed web
var embedded embed.FS

func assetsFS() fs.FS {
	sub, err := fs.Sub(embedded, "web")
	if err != nil {
		panic(err)
	}
	return sub
}
