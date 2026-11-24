package render

import "github.com/alecthomas/chroma/v2"

// railscasts is the dark syntax style, after the Railscasts vim colorscheme
// (Normal #E6E1DC on #141414). Chroma ships no railscasts style.
var railscasts = chroma.MustNewStyle("railscasts", chroma.StyleEntries{
	chroma.Background:            "#e6e1dc bg:#141414",
	chroma.Text:                  "#e6e1dc",
	chroma.Error:                 "#ffc66d bg:#990000",
	chroma.Comment:               "italic #bc9458",
	chroma.CommentPreproc:        "noitalic #cc7833",
	chroma.Keyword:               "#cc7833",
	chroma.KeywordConstant:       "#6d9cbe",
	chroma.KeywordType:           "#da4939",
	chroma.Operator:              "#e6e1dc",
	chroma.OperatorWord:          "#cc7833",
	chroma.Punctuation:           "#e6e1dc",
	chroma.Name:                  "#e6e1dc",
	chroma.NameBuiltin:           "#6d9cbe",
	chroma.NameBuiltinPseudo:     "#ffc66d",
	chroma.NameClass:             "#da4939",
	chroma.NameConstant:          "#6d9cbe",
	chroma.NameDecorator:         "#cc7833",
	chroma.NameException:         "#da4939",
	chroma.NameFunction:          "#ffc66d",
	chroma.NameNamespace:         "#e6e1dc",
	chroma.NameTag:               "#e8bf6a",
	chroma.NameAttribute:         "#ffc66d",
	chroma.NameVariable:          "#d0d0ff",
	chroma.NameLabel:             "#6d9cbe",
	chroma.LiteralString:         "#a5c261",
	chroma.LiteralStringEscape:   "#519f50",
	chroma.LiteralStringInterpol: "#519f50",
	chroma.LiteralStringSymbol:   "#6d9cbe",
	chroma.LiteralNumber:         "#a5c261",
	chroma.GenericDeleted:        "#da4939",
	chroma.GenericInserted:       "#a5c261",
	chroma.GenericHeading:        "bold #ffffff",
	chroma.GenericSubheading:     "#bc9458",
	chroma.GenericEmph:           "italic",
	chroma.GenericStrong:         "bold",
	chroma.GenericPrompt:         "#999999",
})
