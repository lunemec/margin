package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	portalPath = "/org/freedesktop/portal/desktop"
	portalName = "org.freedesktop.portal.Desktop"
)

// systemDark asks the desktop portal for the colour scheme; without a preference there (or a
// portal, as under i3), the GTK theme's name decides.
func systemDark(*application.App) bool {
	conn, err := dbus.SessionBus()
	if err == nil {
		obj := conn.Object(portalName, portalPath)
		var v dbus.Variant
		if obj.Call("org.freedesktop.portal.Settings.Read", 0, "org.freedesktop.appearance", "color-scheme").Store(&v) == nil {
			switch scheme, _ := unwrap(v).(uint32); scheme {
			case 1:
				return true
			case 2:
				return false
			}
		}
		if obj.Call("org.freedesktop.portal.Settings.Read", 0, "org.gnome.desktop.interface", "gtk-theme").Store(&v) == nil {
			if name, ok := unwrap(v).(string); ok && name != "" {
				return darkName(name)
			}
		}
	}
	if t := os.Getenv("GTK_THEME"); t != "" {
		return darkName(t)
	}
	return darkName(gtkSettingsTheme())
}

// Settings.Read wraps the value in a second variant.
func unwrap(v dbus.Variant) any {
	val := v.Value()
	if inner, ok := val.(dbus.Variant); ok {
		return inner.Value()
	}
	return val
}

// darkName: Adwaita-dark, Arc-Dark, Breeze:dark… but not Arc-Darker, whose windows are light.
func darkName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "dark") && !strings.HasSuffix(n, "darker")
}

func gtkSettingsTheme() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	f, err := os.Open(filepath.Join(dir, "gtk-3.0", "settings.ini"))
	if err != nil {
		return ""
	}
	defer f.Close()
	name := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		switch k, v = strings.TrimSpace(k), strings.TrimSpace(v); {
		case !ok:
		case k == "gtk-application-prefer-dark-theme" && (v == "1" || v == "true"):
			return "dark"
		case k == "gtk-theme-name":
			name = v
		}
	}
	return name
}

func watchSystemTheme(ping func()) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	if conn.AddMatchSignal(dbus.WithMatchObjectPath(portalPath), dbus.WithMatchInterface("org.freedesktop.portal.Settings"), dbus.WithMatchMember("SettingChanged")) != nil {
		conn.Close()
		return
	}
	c := make(chan *dbus.Signal, 10)
	conn.Signal(c)
	for s := range c {
		if len(s.Body) < 2 {
			continue
		}
		if key, _ := s.Body[1].(string); key == "color-scheme" || key == "gtk-theme" {
			ping()
		}
	}
}
