package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

const logoArt = `
        _   _        _                  _            _          
       /\_\/\_\ _   /\_\               /\ \         /\ \     _  
      / / / / //\_\/ / /         _    /  \ \       /  \ \   /\_\
     /\ \/ \ \/ / /\ \ \__      /\_\ / /\ \ \     / /\ \ \_/ / /
    /  \____\__/ /  \ \___\    / / // / /\ \ \   / / /\ \___/ / 
   / /\/________/    \__  /   / / // / /  \ \_\ / / /  \/____/  
  / / /\/_// / /     / / /   / / // / /   / / // / /    / / /   
 / / /    / / /     / / /   / / // / /   / / // / /    / / /    
/ / /    / / /     / / /___/ / // / /___/ / // / /    / / /     
\/_/    / / /     / / /____\/ // / /____\/ // / /    / / /      
        \/_/      \/_________/ \/_________/ \/_/     \/_/       `

// LogoLines returns the muon ASCII art logo centered within the given terminal width.
func LogoLines(width int) []string {
	if width < 1 {
		width = 1
	}

	lines := strings.Split(logoArt, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		padding := (width - runewidth.StringWidth(line)) / 2
		if padding < 0 {
			padding = 0
		}
		result = append(result, strings.Repeat(" ", padding)+line)
	}
	return result
}
