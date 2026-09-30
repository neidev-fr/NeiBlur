// NeiBlur — ajoute du motion blur à tes vidéos, simplement.
// Créé par neidev. Moteur de rendu : Blur de f0e (GPL-3.0), https://github.com/f0e/blur
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

const AppVersion = "1.0.0"

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:            "NeiBlur",
		Width:            1180,
		Height:           760,
		MinWidth:         920,
		MinHeight:        620,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 17, G: 18, B: 22, A: 255},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "neidev.neiblur",
			OnSecondInstanceLaunch: app.secondInstance,
		},
		Windows: &windows.Options{
			Theme:                windows.SystemDefault,
			WebviewUserDataPath:  webviewDataDir(),
			DisablePinchZoom:     true,
			WebviewGpuIsDisabled: false,
		},
	})
	if err != nil {
		println("Erreur :", err.Error())
	}
}
