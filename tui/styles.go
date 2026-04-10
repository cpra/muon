package tui

import "github.com/gdamore/tcell/v3"

var (
	baseStyle = tcell.StyleDefault

	logoStyle = tcell.StyleDefault.
			Foreground(tcell.PaletteColor(12)).
			Bold(true)

	userStyle = tcell.StyleDefault.
			Foreground(tcell.PaletteColor(12)).
			Bold(true)

	assistantStyle = tcell.StyleDefault

	toolStyle = tcell.StyleDefault.
			Foreground(tcell.PaletteColor(11))

	toolResultStyle = tcell.StyleDefault.
			Foreground(tcell.PaletteColor(8))

	errorStyle = tcell.StyleDefault.
			Foreground(tcell.PaletteColor(9)).
			Bold(true)

	footerStyle = tcell.StyleDefault.
			Foreground(tcell.PaletteColor(7))

	footerDimStyle = tcell.StyleDefault.
			Foreground(tcell.PaletteColor(8))

	footerCostGreen = tcell.StyleDefault.
			Foreground(tcell.PaletteColor(10))

	footerCostYellow = tcell.StyleDefault.
				Foreground(tcell.PaletteColor(11))

	footerCostOrange = tcell.StyleDefault.
				Foreground(tcell.PaletteColor(208))

	footerCostRed = tcell.StyleDefault.
			Foreground(tcell.PaletteColor(9))

	footerContextGreen = tcell.StyleDefault.
				Foreground(tcell.PaletteColor(10))

	footerContextYellow = tcell.StyleDefault.
				Foreground(tcell.PaletteColor(11))

	footerContextOrange = tcell.StyleDefault.
				Foreground(tcell.PaletteColor(208))

	footerContextRed = tcell.StyleDefault.
				Foreground(tcell.PaletteColor(9))

	inputBorderStyle = tcell.StyleDefault.
				Foreground(tcell.PaletteColor(8))

	inputPromptStyle = tcell.StyleDefault.
				Foreground(tcell.PaletteColor(12)).
				Bold(true)

	inputTextStyle = tcell.StyleDefault

	dimStyle = tcell.StyleDefault.
			Foreground(tcell.PaletteColor(8))
)
