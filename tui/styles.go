package tui

import (
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
)

var (
	baseStyle = tcell.StyleDefault

	logoStyle = tcell.StyleDefault.
			Foreground(color.PaletteColor(3)).
			Bold(true)

	userStyle = tcell.StyleDefault.
			Foreground(color.PaletteColor(4)).
			Bold(true)

	assistantStyle = tcell.StyleDefault

	markdownHeadingStyle = tcell.StyleDefault.
				Foreground(color.PaletteColor(1)).
				Bold(true)

	markdownInlineCodeStyle = tcell.StyleDefault.
				Foreground(color.PaletteColor(2))

	markdownCodeBlockStyle = tcell.StyleDefault.
				Foreground(color.PaletteColor(2))

	markdownTableBorderStyle = tcell.StyleDefault.
					Foreground(color.LightGray)

	markdownTableHeaderStyle = tcell.StyleDefault.
					Foreground(color.PaletteColor(5)).
					Bold(true)

	toolStyle = tcell.StyleDefault.
			Foreground(color.PaletteColor(11))

	toolResultStyle = tcell.StyleDefault.
			Foreground(color.PaletteColor(8))

	errorStyle = tcell.StyleDefault.
			Foreground(color.PaletteColor(9)).
			Bold(true)

	footerStyle = tcell.StyleDefault.
			Foreground(color.PaletteColor(7))

	footerDimStyle = tcell.StyleDefault.
			Foreground(color.PaletteColor(8))

	footerCostGreen = tcell.StyleDefault.
			Foreground(color.PaletteColor(4))

	footerCostYellow = tcell.StyleDefault.
				Foreground(color.PaletteColor(3))

	footerCostOrange = tcell.StyleDefault.
				Foreground(color.PaletteColor(6))

	footerCostRed = tcell.StyleDefault.
			Foreground(color.PaletteColor(1))

	footerContextGreen = tcell.StyleDefault.
				Foreground(color.PaletteColor(4))

	footerContextYellow = tcell.StyleDefault.
				Foreground(color.PaletteColor(3))

	footerContextOrange = tcell.StyleDefault.
				Foreground(color.PaletteColor(6))

	footerContextRed = tcell.StyleDefault.
				Foreground(color.PaletteColor(1))

	inputBorderStyle = tcell.StyleDefault.
				Foreground(color.PaletteColor(8))

	inputPromptStyle = tcell.StyleDefault.
				Foreground(color.PaletteColor(4))

	inputTextStyle = tcell.StyleDefault

	dimStyle = tcell.StyleDefault.
			Foreground(color.PaletteColor(8))
)
