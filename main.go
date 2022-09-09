package main

import (
	"embed"
	"flag"
	"log"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
)

//go:embed frontend/dist
var assets embed.FS

type Config struct {
	Host          string
	LeafEntryType string
	Scene         string
	Envs          []string
	Dir           map[string]string
	Programs      []*Program
}

func mustReadConfig(config string) *Config {
	cfg := &Config{}
	_, err := toml.DecodeFile(config, &cfg)
	if err != nil {
		log.Fatalf("couldn't decode config file: %s", config)
	}
	sort.Slice(cfg.Programs, func(i, j int) bool {
		return cfg.Programs[i].Name < cfg.Programs[j].Name
	})
	return cfg
}

func main() {
	var config string
	flag.StringVar(&config, "config", "config.toml", "path to config file")
	flag.Parse()
	if config == "" {
		log.Fatal("config file path not defined")
	}
	cfg := mustReadConfig(config)

	// Create an instance of the app structure
	app := NewApp(cfg)

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "Canal",
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
