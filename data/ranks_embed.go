package data

import "embed"

// RankIconsFS contains static rank icons used in the embeds.
//
//go:embed ranks/*.png
var RankIconsFS embed.FS
