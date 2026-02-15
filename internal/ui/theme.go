package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type suidaoTheme struct{}

func newSuidaoTheme() fyne.Theme {
	return &suidaoTheme{}
}

func (t *suidaoTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0x0B, G: 0x10, B: 0x16, A: 0xFF}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0xE9, G: 0xEE, B: 0xF4, A: 0xFF}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0x19, G: 0xC4, B: 0xA2, A: 0xFF}
	case theme.ColorNameButton:
		return color.NRGBA{R: 0x17, G: 0x22, B: 0x2D, A: 0xFF}
	case theme.ColorNameHover:
		return color.NRGBA{R: 0x21, G: 0x31, B: 0x40, A: 0xFF}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0x10, G: 0x18, B: 0x22, A: 0xFF}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 0x8A, G: 0x96, B: 0xA3, A: 0xFF}
	case theme.ColorNameError:
		return color.NRGBA{R: 0xFF, G: 0x5D, B: 0x6C, A: 0xFF}
	}
	return theme.DefaultTheme().Color(name, theme.VariantDark)
}

func (t *suidaoTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *suidaoTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *suidaoTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 10
	case theme.SizeNameInlineIcon:
		return 16
	case theme.SizeNameText:
		return 13
	}
	return theme.DefaultTheme().Size(name)
}
