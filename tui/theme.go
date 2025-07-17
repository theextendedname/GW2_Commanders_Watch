package tui

import "github.com/charmbracelet/lipgloss"

// ShadesOfPurple is a Lipgloss color palette for the "Shades of Purple" theme.
// It includes colors for various UI elements and code highlighting.
type ShadesOfPurple struct {
	Background        lipgloss.Color
	Foreground        lipgloss.Color
	LightBlue         lipgloss.Color
	AccentBlue        lipgloss.Color
	AccentPurple      lipgloss.Color
	AccentCyan        lipgloss.Color
	AccentGreen       lipgloss.Color
	AccentYellow      lipgloss.Color
	AccentRed         lipgloss.Color
	Comment           lipgloss.Color
	Gray              lipgloss.Color
	GradientColor1    lipgloss.Color // First color in the gradient
	GradientColor2    lipgloss.Color // Second color in the gradient
	GradientColor3    lipgloss.Color // Third color in the gradient
	AccentYellowAlt   lipgloss.Color
	AccentOrange      lipgloss.Color
	AccentPink        lipgloss.Color
	AccentLightPurple lipgloss.Color
	AccentDarkPurple  lipgloss.Color
	AccentTeal        lipgloss.Color
}

// NewShadesOfPurple creates and returns a new ShadesOfPurple color palette.
func NewShadesOfPurple() ShadesOfPurple {
	return ShadesOfPurple{
		Background:        lipgloss.Color("#2d2b57"),
		Foreground:        lipgloss.Color("#e3dfff"),
		LightBlue:         lipgloss.Color("#847ace"),
		AccentBlue:        lipgloss.Color("#a599e9"),
		AccentPurple:      lipgloss.Color("#ac65ff"),
		AccentCyan:        lipgloss.Color("#a1feff"),
		AccentGreen:       lipgloss.Color("#A5FF90"),
		AccentYellow:      lipgloss.Color("#fad000"),
		AccentRed:         lipgloss.Color("#ff628c"),
		Comment:           lipgloss.Color("#B362FF"),
		Gray:              lipgloss.Color("#726c86"),
		GradientColor1:    lipgloss.Color("#4d21fc"),
		GradientColor2:    lipgloss.Color("#847ace"),
		GradientColor3:    lipgloss.Color("#ff628c"),
		AccentYellowAlt:   lipgloss.Color("#f8d000"),
		AccentOrange:      lipgloss.Color("#fb9e00"),
		AccentPink:        lipgloss.Color("#fa658d"),
		AccentLightPurple: lipgloss.Color("#c991ff"),
		AccentDarkPurple:  lipgloss.Color("#6943ff"),
		AccentTeal:        lipgloss.Color("#2ee2fa"),
	}
}
