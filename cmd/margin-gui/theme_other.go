//go:build !linux

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func systemDark(app *application.App) bool {
	return app != nil && app.Env.IsDarkMode()
}

func watchSystemTheme(func()) {}
