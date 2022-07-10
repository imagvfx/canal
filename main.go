package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
)

//go:embed frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "field",
		Width:     1024,
		Height:    768,
		Assets:    assets,
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
			&Elem{},
			&Entry{},
		},
	})

	if err != nil {
		println("Error:", err)
	}
}
