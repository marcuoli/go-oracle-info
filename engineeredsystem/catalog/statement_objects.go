package catalog

import (
	"strings"

	"github.com/marcuoli/go-oracle-session/diagnostic"
)

// StatementObjects reports the complete reviewed direct-object list for the
// Metadata selection. Pass the original execution error, or nil after success.
func (options MetadataOptions) StatementObjects(executionErr error) ([]diagnostic.ObjectStatus, error) {
	plan, err := metadataDatasetPlan(options)
	return plan.statementObjects(executionErr, err)
}

// StatementObjects reports the selected Cells source, including the outcome of
// capability selection. Use the same admitted options as the execution.
func (options CellsOptions) StatementObjects(executionErr error) ([]diagnostic.ObjectStatus, error) {
	plan, err := cellsDatasetPlan(options)
	return plan.statementObjects(executionErr, err)
}

// StatementObjects reports the reviewed unqualified ConnectedCells reference.
// Its database owner is unresolved on the supplied connection.
func (options ConnectedCellsOptions) StatementObjects(executionErr error) ([]diagnostic.ObjectStatus, error) {
	plan, err := connectedCellsDatasetPlan(options)
	return plan.statementObjects(executionErr, err)
}

// StatementObjects reports the reviewed unqualified VisibleCellCount reference.
// Its database owner is unresolved on the supplied connection.
func (options VisibleCellCountOptions) StatementObjects(executionErr error) ([]diagnostic.ObjectStatus, error) {
	plan, err := visibleCellCountDatasetPlan(options)
	return plan.statementObjects(executionErr, err)
}

// StatementObjects reports the exact Configuration source chosen by the
// supplied variant and admitted capabilities.
func (options ConfigurationOptions) StatementObjects(executionErr error) ([]diagnostic.ObjectStatus, error) {
	plan, err := configurationDatasetPlan(options)
	return plan.statementObjects(executionErr, err)
}

// StatementObjects reports the exact Alerts source chosen by the supplied
// capabilities. Use the original execution error, or nil after success.
func (options AlertsOptions) StatementObjects(executionErr error) ([]diagnostic.ObjectStatus, error) {
	plan, err := alertsDatasetPlan(options)
	return plan.statementObjects(executionErr, err)
}

func (p datasetSelection) statementObjects(executionErr, selectionErr error) ([]diagnostic.ObjectStatus, error) {
	if selectionErr != nil {
		return nil, selectionErr
	}
	// Each reviewed plan directly references one fixed source. The selector's
	// source identifier is authoritative; no submitted SQL or synonym is resolved here.
	object := diagnostic.ObjectRef{Name: p.provenance.Source}
	if owner, name, qualified := strings.Cut(p.provenance.Source, "."); qualified {
		object = diagnostic.ObjectRef{Schema: owner, Name: name, Kind: "view"}
	}
	return diagnostic.StatementObjects(executionErr, []diagnostic.ObjectRef{object})
}
