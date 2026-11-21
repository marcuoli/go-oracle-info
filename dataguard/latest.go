package dataguard

import (
	numtext "github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-session/session"
	"strconv"
)

// LatestRedo retains actual latest timestamp ties and undated Last Applied Redo
// rows for one reporter/container. Comments are raw; they are never parsed as a
// sequence or combined with an independently chosen maximum SCN.
type LatestRedo struct {
	InstanceID, ContainerID session.Value
	Rows, Undated           []RecoveryProgress
}

// LatestLastAppliedRedo groups a complete admitted recovery observation. A NULL
// timestamp remains Undated and cannot establish chronological precedence.
func LatestLastAppliedRedo(result RecoveryProgressResult) ([]LatestRedo, error) {
	if !result.Provenance.Complete {
		return nil, ErrInvalidEvidence
	}
	out := []LatestRedo{}
	groups := map[[2]session.Value]int{}
	times := map[[2]session.Value]string{}
	for _, row := range result.Rows {
		if row.Item.Null || row.Item.Text != "Last Applied Redo" {
			continue
		}
		if !reporter(row.InstanceID, row.ContainerID) {
			return nil, ErrInvalidEvidence
		}
		key := [2]session.Value{row.InstanceID, row.ContainerID}
		i, ok := groups[key]
		if !ok {
			i = len(out)
			groups[key] = i
			out = append(out, LatestRedo{InstanceID: row.InstanceID, ContainerID: row.ContainerID, Rows: []RecoveryProgress{}, Undated: []RecoveryProgress{}})
		}
		if row.Timestamp.Null {
			if row.Timestamp.Text != "" {
				return nil, ErrInvalidEvidence
			}
			out[i].Undated = append(out[i].Undated, row)
			continue
		}
		if _, err := civilTime(row.Timestamp.Text); err != nil {
			return nil, ErrInvalidEvidence
		}
		previous := times[key]
		if row.Timestamp.Text > previous {
			out[i].Rows = []RecoveryProgress{row}
			times[key] = row.Timestamp.Text
		} else if row.Timestamp.Text == previous {
			out[i].Rows = append(out[i].Rows, row)
		}
	}
	return out, nil
}

// ArchiveProgressIdentity groups one logical archived redo identity while
// preserving every destination/reporter contributor. Conflicting compares source
// incarnation/SCN/time metadata; copy-specific applied/deleted/status can differ.
type ArchiveProgressIdentity struct {
	InstanceID, ContainerID, Kind, ThreadNumber, SequenceNumber, ResetlogsChange, ResetlogsTime session.Value
	Copies                                                                                      []ArchiveProgress
	Conflicting                                                                                 bool
}

// ArchiveProgressIdentities groups complete rows without choosing a preferred
// destination or merging reporters. It does not prove full history or gap closure.
func ArchiveProgressIdentities(result ArchiveProgressResult) ([]ArchiveProgressIdentity, error) {
	if !result.Provenance.Complete {
		return nil, ErrInvalidEvidence
	}
	out := []ArchiveProgressIdentity{}
	groups := map[[7]session.Value]int{}
	for _, row := range result.Rows {
		if !reporter(row.InstanceID, row.ContainerID) || row.Kind.Null || (row.Kind.Text != "RECEIVED" && row.Kind.Text != "APPLIED") || row.ThreadNumber.Null || !numtext.IsUnsignedInteger(row.ThreadNumber.Text) || row.SequenceNumber.Null || !numtext.IsUnsignedInteger(row.SequenceNumber.Text) || row.ResetlogsChange.Null || !numtext.IsUnsignedInteger(row.ResetlogsChange.Text) || row.ResetlogsTime.Null {
			return nil, ErrInvalidEvidence
		}
		if _, err := civilTime(row.ResetlogsTime.Text); err != nil {
			return nil, ErrInvalidEvidence
		}
		key := [7]session.Value{row.InstanceID, row.ContainerID, row.Kind, row.ThreadNumber, row.SequenceNumber, row.ResetlogsChange, row.ResetlogsTime}
		i, ok := groups[key]
		if !ok {
			i = len(out)
			groups[key] = i
			out = append(out, ArchiveProgressIdentity{InstanceID: row.InstanceID, ContainerID: row.ContainerID, Kind: row.Kind, ThreadNumber: row.ThreadNumber, SequenceNumber: row.SequenceNumber, ResetlogsChange: row.ResetlogsChange, ResetlogsTime: row.ResetlogsTime, Copies: []ArchiveProgress{}})
		} else {
			first := out[i].Copies[0]
			if first.ResetlogsID != row.ResetlogsID || first.FirstChange != row.FirstChange || first.FirstTime != row.FirstTime || first.NextChange != row.NextChange || first.NextTime != row.NextTime {
				out[i].Conflicting = true
			}
		}
		out[i].Copies = append(out[i].Copies, row)
	}
	return out, nil
}
func reporter(instance, container session.Value) bool {
	n, e := strconv.ParseInt(instance.Text, 10, 64)
	if instance.Null || e != nil || n < 1 {
		return false
	}
	if container.Null {
		return container.Text == ""
	}
	n, e = strconv.ParseInt(container.Text, 10, 64)
	return e == nil && n >= 0
}
