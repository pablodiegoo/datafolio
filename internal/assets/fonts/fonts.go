package fonts

import "embed"

// FS embeds the bundled SIL Open Font License 1.1 TrueType fonts and license notices.
//
//go:embed *.ttf OFL-*.txt
var FS embed.FS

// FamilyMetadata describes a bundled SIL OFL 1.1 font family and its license notice.
type FamilyMetadata struct {
	Name    string
	Files   []string
	License string
}

// Families returns metadata for all bundled font families in deterministic order.
func Families() []FamilyMetadata {
	return []FamilyMetadata{
		{
			Name:    "EB Garamond",
			Files:   []string{"EBGaramond[wght].ttf", "EBGaramond-Italic[wght].ttf"},
			License: "OFL-EBGaramond.txt",
		},
		{
			Name:    "JetBrains Mono",
			Files:   []string{"JetBrainsMono[wght].ttf", "JetBrainsMono-Italic[wght].ttf"},
			License: "OFL-JetBrainsMono.txt",
		},
		{
			Name:    "Source Sans 3",
			Files:   []string{"SourceSans3[wght].ttf", "SourceSans3-Italic[wght].ttf"},
			License: "OFL-SourceSans3.txt",
		},
	}
}
