package releases

import (
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
)

// ResolvePatchPlatform resolves a named platform or a numeric MOS platform
// token (optionally followed by P) for the existing patch selectors. mosLabels
// is optional and maps tokens to labels from the caller's Oracle platform
// listing. A recognized token whose label has no supported catalog name
// returns ErrPatchPackageUnavailable. This does not verify package availability.
func ResolvePatchPlatform(input string, mosLabels map[string]string) (string, error) {
	input = strings.TrimSpace(input)
	if name := namedPatchPlatform(input); name != "" {
		return name, nil
	}

	number := strings.TrimSuffix(input, "P")
	if len(number) > 6 || !numeric.IsUnsignedInteger(number) || number[0] == '0' {
		return "", ErrInvalidPatchPackageQuery
	}
	first, second := mosLabels[number+"P"], mosLabels[number]
	name := namedPatchPlatform(first)
	switch {
	case first == "":
		first, name = second, namedPatchPlatform(second)
	case second != "" && first != second && (name == "" || name != namedPatchPlatform(second)):
		return "", ErrInvalidPatchPackageQuery
	}

	var established string
	switch number {
	case "226":
		established = LinuxX8664
	case "233":
		established = WindowsX64
	}
	if established != "" {
		if first != "" && name != established {
			return "", ErrInvalidPatchPackageQuery
		}
		return established, nil
	}
	if first == "" {
		return "", ErrInvalidPatchPackageQuery
	}
	if name == "" {
		return "", ErrPatchPackageUnavailable
	}
	return name, nil
}

var mosPlatformLabelSlug = strings.NewReplacer(" ", "_", "(", "", ")", "", ":", "")

func namedPatchPlatform(label string) string {
	label = strings.Join(strings.Fields(label), " ")
	if isPatchPlatform(label) {
		return label
	}
	// The MOS dropdown can append a filename-style name to its display label.
	// Both parts must identify the same known platform.
	start := strings.LastIndex(label, " (")
	if start < 0 || !strings.HasSuffix(label, ")") {
		return ""
	}
	name, slug := label[:start], label[start+2:len(label)-1]
	if isPatchPlatform(name) && slug == mosPlatformLabelSlug.Replace(name) {
		return name
	}
	return ""
}
