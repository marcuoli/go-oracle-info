package memory

import (
	"context"
	"database/sql"
	"errors"

	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// CollectRAC reads a previously admitted non-ADB RAC scope through the owned
// reader. It does not discover or admit a backing cluster, retry errors or change
// sessions. The reader must retain complete results and enforce its row budget.
func CollectRAC(ctx context.Context, reader Reader, options Options, ref reference.Reference) (RACObservation, error) {
	unavailable := RACObservation{SGA: Total{State: Unavailable}, PGA: Total{State: Unavailable}}
	if reader == nil || ref.Namespace() == "" {
		return unavailable, errors.New("RAC reader and execution reference are required")
	}
	if err := ctx.Err(); err != nil {
		return unavailable, err
	}
	text, err := RACStatement(options)
	if err != nil {
		return unavailable, err
	}
	var args []any
	if options.Scope == DiscoveredPDB {
		args = []any{sql.Named("target_container_id", options.TargetContainerID)}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: ref, Text: text, Args: args})
	if errors.Is(err, sql.ErrNoRows) {
		return RACObservation{SGA: Total{State: NoRows}, PGA: Total{State: NoRows}}, nil
	}
	if err != nil {
		return unavailable, &ReadError{cause: err}
	}
	return DecodeRAC(result, options)
}
