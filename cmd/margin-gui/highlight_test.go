package main

import (
	"testing"

	"github.com/alecthomas/chroma/v2"

	"github.com/redrick/margin/internal/render"
)

func TestSpanOfThemes(t *testing.T) {
	for _, tt := range []struct {
		name  string
		token chroma.TokenType
		dark  string
		light string
	}{
		{"keyword", chroma.Keyword, "#ff7b72", "var(--cm-keyword)"},
		{"string", chroma.LiteralString, "#a5d6ff", "var(--cm-string)"},
		{"number", chroma.LiteralNumber, "#79c0ff", "var(--cm-number)"},
		{"function", chroma.NameFunction, "#d2a8ff", "var(--cm-function)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sp := spanOf(render.Span{Text: "sample", Type: tt.token, Color: tt.dark, Bold: true})
			if sp.Text != "sample" || sp.Color != tt.dark || !sp.Bold || sp.Light != tt.light || sp.LightBold {
				t.Fatalf("wrong theme styles: %+v", sp)
			}
		})
	}
	if sp := spanOf(render.Span{Type: chroma.CommentSingle, Italic: true}); !sp.Italic || !sp.LightItalic || sp.Light != "var(--cm-comment)" {
		t.Fatalf("comment emphasis was lost: %+v", sp)
	}
}
