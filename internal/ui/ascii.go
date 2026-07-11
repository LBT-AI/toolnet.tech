package ui

// ToolnetASCIIArt returns the large block-letter ASCII art for TOOLNET
func ToolnetASCIIArt() string {
	return `
████████╗ ██████╗  ██████╗ ██╗     ███╗   ██╗███████╗████████╗
╚══██╔══╝██╔═══██╗██╔═══██╗██║     ████╗  ██║██╔════╝╚══██╔══╝
   ██║   ██║   ██║██║   ██║██║     ██╔██╗ ██║█████╗     ██║
   ██║   ██║   ██║██║   ██║██║     ██║╚██╗██║██╔══╝     ██║
   ██║   ╚██████╔╝╚██████╔╝███████╗██║ ╚████║███████╗   ██║
   ╚═╝    ╚═════╝  ╚═════╝ ╚══════╝╚═╝  ╚═══╝╚══════╝   ╚═╝`
}

// GlobeIcon returns a small ASCII globe icon
func GlobeIcon() string {
	return `
    ╭────────╮
   ╱   ╭──╮   ╲
  │   ╱    ╲    │
  │  │  🌐  │   │
  │   ╲    ╱    │
   ╲   ╰──╯   ╱
    ╰────────╯`
}

// LogoCombined returns the full splash with art + globe side by side conceptually
func LogoCombined() string {
	return ToolnetASCIIArt() + "\n" + GlobeIcon()
}

// ToolnetASCIIArtBlock returns the chunky block-letter variant of the TOOLNET
// logo (plain, uncolored). Kept alongside ToolnetASCIIArt (serif variant) so
// both styles are available.
func ToolnetASCIIArtBlock() string {
	return `██████████  ██████████  ██████████  ██    █  ██    ██  ████████  ██████████
██          ██      ██  ██      ██  ██    █  ███   ██  ██        ██
██          ██      ██  ██      ██  ██    █  ██ █  ██  ████████  ██
██          ██      ██  ██      ██  ██    █  ██  █ ██  ██        ██
██          ██████████  ██████████  ██████  ██   ███  ████████  ██`
}
