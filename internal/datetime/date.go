// Package datetime builds reviewed Oracle civil-date SQL expressions.
package datetime

// GregorianDate uses Gregorian EXTRACT fields rather than session NLS_CALENDAR.
// Oracle DATE has whole seconds and no timezone; NULL stays SQL NULL.
// The expression must be a fixed reviewed DATE source, never caller-supplied SQL.
func GregorianDate(expression string) string {
	return gregorianCivil(expression, "TIMESTAMP", "FM00", "")
}

// GregorianTimestamp6 preserves the six fractional digits of a reviewed
// TIMESTAMP(6) source without introducing a timezone or relying on NLS_CALENDAR.
func GregorianTimestamp6(expression string) string {
	return gregorianCivil(expression, "TIMESTAMP", "FM00D000000", "")
}

// GregorianTimestamp9 preserves the nine fractional digits of a reviewed
// TIMESTAMP(9) source, including values beyond microsecond precision.
func GregorianTimestamp9(expression string) string {
	return gregorianCivil(expression, "TIMESTAMP(9)", "FM00D000000000", "")
}

// GregorianUTC9 converts a fixed reviewed TIMESTAMP WITH TIME ZONE expression to
// UTC, preserving fractional seconds and adding the Z designator only for a value.
func GregorianUTC9(expression string) string {
	return gregorianCivil("SYS_EXTRACT_UTC("+expression+")", "TIMESTAMP(9)", "FM00D000000000", " || 'Z'")
}

func gregorianCivil(expression, cast, secondFormat, suffix string) string {
	stamp := "CAST(" + expression + " AS " + cast + ")"
	fields := []struct{ name, format string }{{"YEAR", "FM0000"}, {"MONTH", "FM00"}, {"DAY", "FM00"}, {"HOUR", "FM00"}, {"MINUTE", "FM00"}, {"SECOND", secondFormat}}
	separators := []string{"-", "-", "T", ":", ":"}
	text := "CASE WHEN " + expression + " IS NULL THEN NULL ELSE "
	for i, field := range fields {
		if i > 0 {
			text += " || '" + separators[i-1] + "' || "
		}
		text += "TO_CHAR(EXTRACT(" + field.name + " FROM " + stamp + "), '" + field.format + "', 'NLS_NUMERIC_CHARACTERS=''.,''')"
	}
	return text + suffix + " END"
}
