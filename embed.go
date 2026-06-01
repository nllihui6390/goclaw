package main

import (
	"embed"
	"io/fs"
	"net/http"

	"go-claw/server"
)

//go:embed frontend/dist
var embeddedFrontend embed.FS

func initFrontend() {
	sub, err := fs.Sub(embeddedFrontend, "frontend/dist")
	if err != nil {
		return
	}
	server.FrontendFS = http.FS(sub)
}
