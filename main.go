package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/sya-ri/minecraft-proxy/internal/relay"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var frontend embed.FS

var googleOAuthClientID, cloudflareOAuthClientID string

func main() {
	assets, err := fs.Sub(frontend, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	service, err := relay.NewDesktop(googleOAuthClientID, cloudflareOAuthClientID)
	if err != nil {
		log.Fatal(err)
	}
	app := application.New(application.Options{
		Name:           "Minecraft Proxy",
		SingleInstance: &application.SingleInstanceOptions{UniqueID: "dev.sya-ri.minecraft-proxy"},
		Services:       []application.Service{application.NewService(service)},
		Assets:         application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:            application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Minecraft Proxy", Width: 760, Height: 680, MinWidth: 500, MinHeight: 470,
		URL: "/", BackgroundColour: application.NewRGB(242, 244, 238),
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
