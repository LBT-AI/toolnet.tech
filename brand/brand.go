package brand

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	Reset  = "\x1b[0m"
	Cyan   = "\x1b[38;2;0;200;220m"
	Yellow = "\x1b[38;2;255;215;0m"
	White  = "\x1b[37m"
	Gray   = "\x1b[90m"
)

// bannerInner is the inner content width (between the two │ borders).
const bannerInner = 76

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// visualLen returns the displayed width of s, ignoring ANSI escape codes.
func visualLen(s string) int {
	return len([]rune(ansiRe.ReplaceAllString(s, "")))
}

// frame wraps inner text with the box side borders, padding with spaces so
// every line has the same visual width regardless of embedded ANSI codes.
func frame(inner string) string {
	v := visualLen(inner)
	if v > bannerInner {
		v = bannerInner
	}
	return "│" + inner + strings.Repeat(" ", bannerInner-v) + "│"
}

// Banner returns the LETAN MEDIA / TOOLNET CLI brand banner.
func Banner() string {
	var b strings.Builder

	b.WriteString(White + "┌" + strings.Repeat("─", bannerInner) + "┐\n" + Reset)

	b.WriteString(White + frame("  Welcome to LETAN MEDIA") + "\n" + Reset)
	b.WriteString(White + frame("") + "\n" + Reset)

	// ── TOOLNET block letters (5 rows, cyan) ──
	toolnetRows := []string{
		Cyan + "██████████  ██████████  ██████████  ██    █  ██    ██  ████████  ██████████" + Reset,
		Cyan + "██          ██      ██  ██      ██  ██    █  ███   ██  ██        ██        " + Reset,
		Cyan + "██          ██      ██  ██      ██  ██    █  ██ █  ██  ████████  ██        " + Reset,
		Cyan + "██          ██      ██  ██      ██  ██    █  ██  █ ██  ██        ██        " + Reset,
		Cyan + "██          ██████████  ██████████  ██████  ██   ███  ████████  ██        " + Reset,
	}
	for _, r := range toolnetRows {
		b.WriteString(White + frame(" "+r) + "\n" + Reset)
	}

	b.WriteString(White + frame("") + "\n" + Reset)

	// ── Globe + Coin icons (centered) ──
	iconRows := [][2]string{
		{
			Cyan + "    ████████████████    " + Reset,
			Yellow + "    ████████████████    " + Reset,
		},
		{
			Cyan + "  ████  ██  ██  ████    " + Reset,
			Yellow + "  ████   ██████   ████  " + Reset,
		},
		{
			Cyan + " ████  ████  ████  ████  " + Reset,
			Yellow + " ████   ██  ██  ██   ████ " + Reset,
		},
		{
			Cyan + " ██  ████  ██  ████  ██  " + Reset,
			Yellow + " ████  ████  ████  ████ " + Reset,
		},
		{
			Cyan + "███  ██  ██████  ██  ███ " + Reset,
			Yellow + " ████  ██████████  ████ " + Reset,
		},
		{
			Cyan + " ██  ██  ██  ██  ██  ██  " + Reset,
			Yellow + " ████  ██████████  ████ " + Reset,
		},
		{
			Cyan + "  ████  ██  ██  ████    " + Reset,
			Yellow + "  ████   ██████   ████  " + Reset,
		},
		{
			Cyan + "    ████████████████    " + Reset,
			Yellow + "    ████████████████    " + Reset,
		},
	}
	for _, row := range iconRows {
		b.WriteString(White + frame("             "+row[0]+"  "+row[1]+"             ") + "\n" + Reset)
	}

	b.WriteString(White + frame("") + "\n" + Reset)
	b.WriteString(White + "└" + strings.Repeat("─", bannerInner) + "┘\n" + Reset)

	return b.String()
}

// PrintBanner prints the banner to stdout
func PrintBanner() {
	fmt.Print(Banner())
}

// PrintBannerWithTagline prints banner + gray tagline
func PrintBannerWithTagline(tagline string) {
	fmt.Print(Banner())
	if tagline != "" {
		fmt.Println(Gray + "  " + tagline + Reset)
	}
}
