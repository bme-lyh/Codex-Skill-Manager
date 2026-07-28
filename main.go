package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:             "Codex Skill Manager",
		Width:             1360,
		Height:            860,
		MinWidth:          1080,
		MinHeight:         680,
		Frameless:         false,
		BackgroundColour:  &options.RGBA{R: 244, G: 247, B: 252, A: 1},
		AssetServer:       &assetserver.Options{Assets: assets},
		OnStartup:         app.startup,
		OnShutdown:        app.shutdown,
		Bind:              []interface{}{app},
		EnableDefaultContextMenu: false,
	})
	if err != nil {
		log.Fatal(err)
	}
}
