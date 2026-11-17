// Package datetime builds reviewed Oracle civil-date SQL expressions.
package datetime

// GregorianDate uses Gregorian EXTRACT fields rather than session NLS_CALENDAR.
// Oracle DATE has whole seconds and no timezone; NULL stays SQL NULL.
// The expression must be a fixed reviewed DATE source, never caller-supplied SQL.
func GregorianDate(expression string) string {
	stamp := "CAST(" + expression + " AS TIMESTAMP)"
	fields := []struct{ name, format string }{{"YEAR", "FM0000"}, {"MONTH", "FM00"}, {"DAY", "FM00"}, {"HOUR", "FM00"}, {"MINUTE", "FM00"}, {"SECOND", "FM00"}}
	separators := []string{"-", "-", "T", ":", ":"}
	text := "CASE WHEN " + expression + " IS NULL THEN NULL ELSE "
	for i, field := range fields {
		if i > 0 {
			text += " || '" + separators[i-1] + "' || "
		}
		text += "TO_CHAR(EXTRACT(" + field.name + " FROM " + stamp + "), '" + field.format + "', 'NLS_NUMERIC_CHARACTERS=''.,''')"
	}
	return text + " END"
}
