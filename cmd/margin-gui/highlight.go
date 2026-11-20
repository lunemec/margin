package main

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/redrick/margin/internal/render"
)

var latte = styles.Get("catppuccin-latte")

// CSS owns the readable Latte palette shared by review spans and editors.
var latteColors = map[string]string{
	"#4c4f69": "var(--sx-fg)",
	"#d20f39": "var(--issue)",
	"#8839ef": "var(--cm-keyword)",
	"#fe640b": "var(--cm-number)",
	"#179299": "var(--syntax-operator)",
	"#df8e1d": "var(--cm-type)",
	"#1e66f5": "var(--cm-function)",
	"#04a5e5": "var(--syntax-operator)",
	"#dc8a78": "var(--syntax-variable)",
	"#40a02b": "var(--cm-string)",
	"#9ca0b0": "var(--cm-comment)",
	"#acb0be": "var(--cm-comment)",
	"#8c8fa1": "var(--fg-faint)",
}

func spanOf(s render.Span) Span {
	light := latte.Get(s.Type)
	sp := Span{Text: s.Text, Color: s.Color, Bold: s.Bold, Italic: s.Italic,
		LightBold: light.Bold == chroma.Yes, LightItalic: light.Italic == chroma.Yes}
	if light.Colour.IsSet() {
		sp.Light = light.Colour.String()
	}
	if color, ok := latteColors[sp.Light]; ok {
		sp.Light = color
	}
	return sp
}
