package identity

import (
	"context"
	"database/sql"
)

// BorrowedSampleIdentity records the seven raw Oracle values from one
// executing database session. Version is VERSION_FULL and the session context
// values retain their original spelling.
type BorrowedSampleIdentity struct {
	Version, User, IsDBA, Container, DatabaseRole, OpenMode, Protocol string
}

const borrowedSampleIdentitySQL = `SELECT VERSION_FULL AS VERSION_FULL,
    SYS_CONTEXT('USERENV','SESSION_USER') AS SESSION_USER,
    SYS_CONTEXT('USERENV','ISDBA') AS ISDBA,
    SYS_CONTEXT('USERENV','CON_NAME') AS CON_NAME,
    DATABASE_ROLE AS DATABASE_ROLE,
    OPEN_MODE AS OPEN_MODE,
    SYS_CONTEXT('USERENV','NETWORK_PROTOCOL') AS NETWORK_PROTOCOL
FROM V$INSTANCE CROSS JOIN V$DATABASE`

// ReadBorrowedSampleIdentity reads the current identity once on the caller's
// admitted, exclusive connection using ctx. The projection requires
// V$INSTANCE.VERSION_FULL and access to both views. It never substitutes
// VERSION or opens another connection.
//
// It returns a zero identity and the original native query or scan error on
// failure, including sql.ErrNoRows and errors converting SQL NULL to string.
// The caller retains the lease and decides how to handle unsupported Oracle
// metadata or cancellation. The query's cursor is closed before return.
func ReadBorrowedSampleIdentity(ctx context.Context, conn *sql.Conn) (BorrowedSampleIdentity, error) {
	if err := ctx.Err(); err != nil {
		return BorrowedSampleIdentity{}, err
	}
	if conn == nil {
		return BorrowedSampleIdentity{}, ErrInvalidRequest
	}

	var id BorrowedSampleIdentity
	if err := conn.QueryRowContext(ctx, borrowedSampleIdentitySQL).Scan(
		&id.Version, &id.User, &id.IsDBA, &id.Container, &id.DatabaseRole, &id.OpenMode, &id.Protocol,
	); err != nil {
		return BorrowedSampleIdentity{}, err
	}
	if err := ctx.Err(); err != nil {
		return BorrowedSampleIdentity{}, err
	}
	return id, nil
}
