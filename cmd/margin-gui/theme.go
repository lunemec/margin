package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/redrick/margin/internal/config"
)

type ThemeView struct {
	Mode string `json:"mode"`
	Dark bool   `json:"dark"`
}

// Theme is the look in effect: the setting, or the system's when there is none.
func (s *Settings) Theme() ThemeView {
	mode := config.Load().Theme
	return ThemeView{Mode: mode, Dark: mode == "dark" || mode == "" && systemDark(s.app)}
}

func (s *Settings) SystemDark() bool { return systemDark(s.app) }

// watchTheme tells the window when the system switches between light and dark.
func watchTheme(app *application.App) {
	ping := func() { app.Event.Emit("theme:system") }
	app.Event.OnApplicationEvent(events.Common.ThemeChanged, func(*application.ApplicationEvent) { ping() })
	go watchSystemTheme(ping)
}

func windowBackground(dark bool) application.RGBA {
	if dark {
		return application.NewRGB(20, 20, 20) // railscasts #141414, matches --bg
	}
	return application.NewRGB(255, 255, 255)
}
