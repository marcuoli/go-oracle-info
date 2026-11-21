// Package numeric validates the textual form of exact numeric facts.
package numeric

// IsUnsignedInteger reports whether text is a nonempty sequence of ASCII decimal
// digits. Leading zeros are allowed and there is no magnitude limit.
func IsUnsignedInteger(text string) bool {
	if text == "" {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

// IsSignedInteger reports whether text is an ASCII decimal integer with an
// optional leading plus or minus sign.
func IsSignedInteger(text string) bool {
	if text != "" && (text[0] == '+' || text[0] == '-') {
		text = text[1:]
	}
	return IsUnsignedInteger(text)
}
