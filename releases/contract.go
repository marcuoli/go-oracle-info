package releases

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
)

const maxReleaseInputBytes = 1 << 20

func readReleaseJSON(reader io.Reader, result any) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("missing JSON input")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxReleaseInputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxReleaseInputBytes {
		return nil, errors.New("JSON input exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		return nil, err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("unexpected trailing JSON input")
	}
	return raw, nil
}

func validOracleSupportCapture(doc Source, capturedUTC, archiveCommit string) bool {
	checked, err := time.Parse(time.RFC3339Nano, doc.CheckedUTC)
	if err != nil || !isOracleSupportArticleID(doc.ID) || strings.TrimSpace(doc.Title) == "" || !isHexLength(doc.SHA256, 64) {
		return false
	}
	if doc.OldDocID != "" && !isLegacyOracleSupportID(doc.OldDocID) {
		return false
	}
	if archiveCommit != "" && !isHexLength(archiveCommit, 40) {
		return false
	}
	if capturedUTC == "" {
		if archiveCommit == "" {
			return false
		}
	} else {
		captured, err := time.Parse(time.RFC3339Nano, capturedUTC)
		if err != nil || captured.After(checked) {
			return false
		}
	}
	return matchingOracleSupportArticle(doc)
}

func isHexLength(value string, digits int) bool {
	if len(value) != digits {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func isOracleSupportArticleID(id string) bool {
	if digits, found := strings.CutPrefix(id, "KB"); found {
		return len(digits) <= 12 && numeric.IsUnsignedInteger(digits)
	}
	return isLegacyOracleSupportID(id)
}

func isLegacyOracleSupportID(id string) bool {
	number, suffix, ok := strings.Cut(id, ".")
	return ok && len(number) <= 12 && len(suffix) <= 4 &&
		numeric.IsUnsignedInteger(number) && numeric.IsUnsignedInteger(suffix)
}
func matchingOracleSupportArticle(doc Source) bool {
	ref, err := url.Parse(doc.URL)
	if err != nil || ref.Scheme != "https" || ref.Host != "support.oracle.com" || ref.User != nil || ref.Fragment != "" {
		return false
	}
	query, err := url.ParseQuery(ref.RawQuery)
	if err != nil || len(query) != 1 {
		return false
	}
	switch ref.Path {
	case "/support/":
		values := query["kmContentId"]
		oldID, _, _ := strings.Cut(doc.OldDocID, ".")
		id, _, _ := strings.Cut(doc.ID, ".")
		return len(values) == 1 && values[0] != "" && (values[0] == id || values[0] == oldID)
	case "/epmos/faces/DocumentDisplay":
		values := query["id"]
		return len(values) == 1 && (values[0] == doc.ID || (doc.OldDocID != "" && values[0] == doc.OldDocID))
	default:
		return false
	}
}
