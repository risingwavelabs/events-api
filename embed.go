package root

import (
	"bytes"
	"embed"
)

//go:embed VERSION
var versionFile embed.FS

var Version string

func init() {
	data, err := versionFile.ReadFile("VERSION")
	if err != nil {
		panic(err)
	}
	data = bytes.TrimSpace(data)
	Version = string(data)
}
