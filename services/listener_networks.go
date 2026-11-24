package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ListenerNetworksOptions carries the observed numeric database release.
type ListenerNetworksOptions struct {
	OracleVersion string
}

// ListenerNetworksRequest adds the caller-owned reference and bounded read limits.
type ListenerNetworksRequest struct {
	Options   ListenerNetworksOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// ListenerNetwork preserves a remote listener registration target. Oracle can
// report an IP address here even when the configured target used a DNS name.
type ListenerNetwork struct {
	InstanceID  session.Value  `json:"instance_id"`
	Network     session.Value  `json:"network"`
	Value       session.Value  `json:"value"`
	ContainerID *session.Value `json:"container_id,omitempty"`
}

// ListenerNetworksResult contains visible remote listener rows and provenance.
type ListenerNetworksResult struct {
	Provenance DatasetProvenance `json:"provenance"`
	Rows       []ListenerNetwork `json:"rows"`
}

func scanMajor(version string) (int, error) {
	major, ok := oracleversion.LegacyFactMajor(version)
	numbers, _ := oracleversion.Parse(version)
	if !ok || major < 11 || (major == 11 && numbers[1] < 2) {
		return 0, ErrUnsupportedVersion
	}
	return major, nil
}

// ListenerNetworksStatement selects visible remote listener registration evidence.
func ListenerNetworksStatement(options ListenerNetworksOptions) (string, error) {
	plan, err := listenerNetworksPlan(options)
	return plan.sql, err
}

func listenerNetworksPlan(options ListenerNetworksOptions) (datasetSelection, error) {
	major, err := scanMajor(options.OracleVersion)
	if err != nil {
		return datasetSelection{}, err
	}
	p := datasetSelection{
		sql: "SELECT v.INST_ID AS INST_ID, v.NETWORK AS NETWORK, v.VALUE AS VALUE",
		schema: []rowset.Column{
			{Name: "INST_ID", Kind: rowset.Integer},
			{Name: "NETWORK", Kind: rowset.Text},
			{Name: "VALUE", Kind: rowset.Text},
		},
		provenance: DatasetProvenance{Source: "GV$LISTENER_NETWORK", OracleMajor: major, Scope: "visible_remote_listeners"},
	}
	if major >= 12 {
		p.sql += ", v.CON_ID AS CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
	}
	p.sql += " FROM GV$LISTENER_NETWORK v WHERE v.TYPE = 'REMOTE LISTENER'"
	if major >= 12 {
		p.sql += " AND (v.CON_ID=0 OR v.CON_ID=TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))) ORDER BY v.INST_ID, v.CON_ID, v.NETWORK, v.VALUE"
	} else {
		p.sql += " ORDER BY v.INST_ID, v.NETWORK, v.VALUE"
	}
	return p, nil
}

// DecodeListenerNetworks validates the entire projection before returning any rows.
func DecodeListenerNetworks(result session.Result, options ListenerNetworksOptions) (ListenerNetworksResult, error) {
	p, err := listenerNetworksPlan(options)
	if err != nil {
		return ListenerNetworksResult{}, err
	}
	return decodeListenerNetworks(result, p)
}

func decodeListenerNetworks(result session.Result, p datasetSelection) (ListenerNetworksResult, error) {
	index, err := datasetColumns(result, p, "INST_ID")
	if err != nil {
		return ListenerNetworksResult{}, err
	}
	rows := make([]ListenerNetwork, len(result.Rows))
	for i, row := range result.Rows {
		id := row[index["INST_ID"]]
		n, err := strconv.ParseInt(id.Text, 10, 64)
		if id.Null || err != nil || n <= 0 {
			return ListenerNetworksResult{}, ErrInvalidEvidence
		}
		item := ListenerNetwork{InstanceID: id, Network: row[index["NETWORK"]], Value: row[index["VALUE"]]}
		if col, ok := index["CON_ID"]; ok {
			container := row[col]
			n, err := strconv.ParseInt(container.Text, 10, 64)
			if container.Null || err != nil || n < 0 {
				return ListenerNetworksResult{}, ErrInvalidEvidence
			}
			item.ContainerID = &container
		}
		rows[i] = item
	}
	return ListenerNetworksResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadListenerNetworks queries once through a caller-admitted, complete reader.
func ReadListenerNetworks(ctx context.Context, reader Reader, request ListenerNetworksRequest) (ListenerNetworksResult, error) {
	p, err := listenerNetworksPlan(request.Options)
	if err != nil {
		return ListenerNetworksResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ListenerNetworksResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return ListenerNetworksResult{}, err
	}
	if err := datasetReadError(ctx, p, nil); err != nil {
		return ListenerNetworksResult{}, err
	}
	result, queryErr := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql})
	if err := datasetReadError(ctx, p, queryErr); err != nil {
		// Classification is safe only when the borrowed cursor failed to open.
		// An owned buffered error can also describe iteration or closure.
		if opening, ok := queryErr.(*rowset.Error); ok && opening.Operation == "query" {
			err = projection.ClassifySourceError(err)
		}
		return ListenerNetworksResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return ListenerNetworksResult{}, datasetReadError(ctx, p, rowset.ErrCapacity)
	}
	facts, decodeErr := decodeListenerNetworks(result, p)
	if err := datasetReadError(ctx, p, decodeErr); err != nil {
		return ListenerNetworksResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ListenerNetworksRows returns the actual native cursor. Its caller owns iteration,
// terminal errors, closure and limits.
func ListenerNetworksRows(ctx context.Context, q rowset.Queryer, options ListenerNetworksOptions) (*sql.Rows, error) {
	p, err := listenerNetworksPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, projection.ClassifySourceError(&DatasetError{p.provenance.Source, err})
	}
	return rows, nil
}

// ListenerNetworksJSON consumes and validates a separate complete bounded cursor.
func ListenerNetworksJSON(ctx context.Context, q rowset.Queryer, options ListenerNetworksOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := listenerNetworksPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeListenerNetworks(result, p)
		return err
	})
}
