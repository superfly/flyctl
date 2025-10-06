package iostreams

import (
	"fmt"
	"github.com/samber/lo"
	"math"
	"os"
	"strings"

	"github.com/mgutz/ansi"
)

var (
	magenta   = ansi.ColorFunc("magenta")
	cyan      = ansi.ColorFunc("cyan")
	red       = ansi.ColorFunc("red")
	yellow    = ansi.ColorFunc("yellow")
	blue      = ansi.ColorFunc("blue")
	green     = ansi.ColorFunc("green")
	gray      = ansi.ColorFunc("black+h")
	bold      = ansi.ColorFunc("default+b")
	underline = ansi.ColorFunc("default+u")
	cyanBold  = ansi.ColorFunc("cyan+b")

	gray256 = func(t string) string {
		return fmt.Sprintf("\x1b[%d;5;%dm%s\x1b[m", 38, 242, t)
	}
	italic = func(t string) string {
		return fmt.Sprintf("\x1b[%dm%s\x1b[m", 3, t)
	}
)

func EnvColorDisabled() bool {
	return os.Getenv("NO_COLOR") != "" || os.Getenv("CLICOLOR") == "0"
}

func EnvColorForced() bool {
	return os.Getenv("CLICOLOR_FORCE") != "" && os.Getenv("CLICOLOR_FORCE") != "0"
}

func Is256ColorSupported() bool {
	term := os.Getenv("TERM")
	colorterm := os.Getenv("COLORTERM")

	return strings.Contains(term, "256") ||
		strings.Contains(term, "24bit") ||
		strings.Contains(term, "truecolor") ||
		strings.Contains(colorterm, "256") ||
		strings.Contains(colorterm, "24bit") ||
		strings.Contains(colorterm, "truecolor")
}

func IsTrueColor() bool {
	term := os.Getenv("TERM")
	colorterm := os.Getenv("COLORTERM")

	return strings.Contains(term, "24bit") ||
		strings.Contains(term, "truecolor") ||
		strings.Contains(colorterm, "24bit") ||
		strings.Contains(colorterm, "truecolor")
}

func NewColorScheme(enabled, is256enabled, trueColor bool) *ColorScheme {
	return &ColorScheme{
		enabled:      enabled,
		is256enabled: is256enabled,
		trueColor:    trueColor,
	}
}

type ColorScheme struct {
	enabled      bool
	is256enabled bool
	trueColor    bool
}

func (c *ColorScheme) Bold(t string) string {
	if !c.enabled {
		return t
	}
	return bold(t)
}

func (c *ColorScheme) Underline(t string) string {
	if !c.enabled {
		return t
	}
	return underline(t)
}

func (c *ColorScheme) Red(t string) string {
	if !c.enabled {
		return t
	}
	return red(t)
}

func (c *ColorScheme) Yellow(t string) string {
	if !c.enabled {
		return t
	}
	return yellow(t)
}

func (c *ColorScheme) Green(t string) string {
	if !c.enabled {
		return t
	}
	return green(t)
}

func (c *ColorScheme) Gray(t string) string {
	if !c.enabled {
		return t
	}
	if c.is256enabled {
		return gray256(t)
	}
	return gray(t)
}

func (c *ColorScheme) Magenta(t string) string {
	if !c.enabled {
		return t
	}
	return magenta(t)
}

func (c *ColorScheme) Cyan(t string) string {
	if !c.enabled {
		return t
	}
	return cyan(t)
}

func (c *ColorScheme) CyanBold(t string) string {
	if !c.enabled {
		return t
	}
	return cyanBold(t)
}

func (c *ColorScheme) Blue(t string) string {
	if !c.enabled {
		return t
	}
	return blue(t)
}

func (c *ColorScheme) Italic(t string) string {
	if !c.enabled {
		return t
	}
	return italic(t)
}

func (c *ColorScheme) SuccessIcon() string {
	return c.SuccessIconWithColor(c.Green)
}

func (c *ColorScheme) SuccessIconWithColor(colo func(string) string) string {
	return colo("✓")
}

func (c *ColorScheme) WarningIcon() string {
	return c.Yellow("!")
}

func (c *ColorScheme) FailureIcon() string {
	return c.Red("X")
}

func (c *ColorScheme) ColorFromString(s string) func(string) string {
	s = strings.ToLower(s)
	var fn func(string) string
	switch s {
	case "bold":
		fn = c.Bold
	case "red":
		fn = c.Red
	case "yellow":
		fn = c.Yellow
	case "green":
		fn = c.Green
	case "gray":
		fn = c.Gray
	case "magenta":
		fn = c.Magenta
	case "cyan":
		fn = c.Cyan
	case "blue":
		fn = c.Blue
	default:
		fn = func(s string) string {
			return s
		}
	}

	return fn
}

// RedGreenGradient wraps a string in an ANSI red-green color gradient at a value between 0-1.
func (c *ColorScheme) RedGreenGradient(s string, value float64) string {
	if !c.enabled {
		return s
	}
	value = lo.Clamp(value, 0, 1)
	if c.trueColor {
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m",
			int(math.Min(255, 2*255*(1-value))),
			int(math.Min(255, 2*255*value)),
			0,
			s,
		)
	}
	colors := []string{"red", "yellow+h", "green", "green+h", "green+bh"}
	if c.is256enabled {
		colors = []string{"196", "202", "208", "214", "220", "190", "154", "118", "82", "46"}
	}
	return ansi.Color(s, colors[int(float64(len(colors)-1)*value)])
}
