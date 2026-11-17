package oracleinfo

import (
	"context"
	"database/sql"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/session"
)

// Exec only forwards the fixed domain statement's native binds to the borrowed
// handle. It owns no transaction/connection and never retries execution.
func (r nativeIdentityReader) Exec(ctx context.Context, statement session.Statement, _ session.ExecOptions) (sql.Result, error) {
	executor, ok := r.queryer.(interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	})
	if !ok {
		return nil, identity.ErrOracleHomeExecutorUnavailable
	}
	return executor.ExecContext(ctx, statement.Text, statement.Args...)
}
