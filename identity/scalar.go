package identity

import "github.com/marcuoli/go-oracle-session/session"

// NameStatement reads only the database name. The established projection is
// deliberately unaliased so a scalar read does not require other identity facts.
func NameStatement() string {
	return `SELECT NAME FROM V$DATABASE`
}

// RoleStatement preserves the established scalar projection and its whitespace.
// It does not probe the platform or other columns selected by DatabaseStatement.
func RoleStatement() string {
	return "SELECT DATABASE_ROLE \n\t\t\t\t\tFROM V$DATABASE"
}

// DecodeName validates a complete scalar NAME result and retains its exact text,
// SQL NULL or authoritative no-row state. Display normalization belongs to callers.
func DecodeName(result session.Result) (Text, error) {
	return decodeScalar(result, "NAME")
}

// DecodeRole validates a complete scalar DATABASE_ROLE result without normalizing
// or inferring a role. It preserves SQL NULL and no-row states independently.
func DecodeRole(result session.Result) (Text, error) {
	return decodeScalar(result, "DATABASE_ROLE")
}

func decodeScalar(result session.Result, column string) (Text, error) {
	r, absent, err := row(result, column)
	if err != nil {
		return Text{}, err
	}
	if absent {
		return Text{State: NoRows}, nil
	}
	return text(r[column]), nil
}
