// Command goldengate-discovery-selectors emits reviewed SQL metadata as JSON.
// It opens no connection. Optional catalog input is a caller-supplied complete
// session.Result JSON object containing the accessible dictionary projection.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"os"

	"github.com/marcuoli/go-oracle-info/goldengate"
	"github.com/marcuoli/go-oracle-session/session"
)

func main() {
	version := flag.String("oracle-version", "", "observed five-part numeric Oracle version")
	catalogPath := flag.String("catalog", "", "optional admitted catalog session.Result JSON file")
	flag.Parse()
	type selected struct {
		Name string `json:"name"`
		SQL  string `json:"sql"`
		Args []any  `json:"args"`
	}
	selections := make([]selected, 0)
	add := func(name, sql string, args []any, err error) {
		if err != nil {
			fail()
		}
		selections = append(selections, selected{Name: name, SQL: sql, Args: args})
	}
	for _, visibility := range []goldengate.DictionaryVisibility{goldengate.AccessibleDictionary, goldengate.DatabaseDictionary} {
		o := goldengate.ProcessOptions{OracleVersion: *version, Visibility: visibility}
		sql, args, err := goldengate.ExtractsStatement(o)
		add("extracts_"+string(visibility), sql, args, err)
		sql, args, err = goldengate.ReplicatsStatement(o)
		add("replicats_"+string(visibility), sql, args, err)
	}
	o := goldengate.HeartbeatCatalogOptions{OracleVersion: *version}
	sql, args, err := goldengate.HeartbeatCatalogStatement(o)
	add("catalog", sql, args, err)
	if *catalogPath != "" {
		f, err := os.Open(*catalogPath)
		if err != nil {
			fail()
		}
		payload, readErr := io.ReadAll(io.LimitReader(f, (8<<20)+1))
		closeErr := f.Close()
		if readErr != nil || closeErr != nil || len(payload) > 8<<20 {
			fail()
		}
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.DisallowUnknownFields()
		var result session.Result
		err = decoder.Decode(&result)
		var extra any
		end := decoder.Decode(&extra)
		if err != nil || end != io.EOF {
			fail()
		}
		catalog, err := goldengate.DecodeHeartbeatCatalog(result, o)
		if err != nil {
			fail()
		}
		for _, object := range catalog.Rows {
			if !object.Supported {
				continue
			}
			source, err := object.Source()
			if err != nil {
				fail()
			}
			if source.Name() == "GG_HEARTBEAT" {
				sql, args, err = goldengate.HeartbeatsStatement(goldengate.HeartbeatsOptions{Source: source})
			} else {
				sql, args, err = goldengate.HeartbeatLagStatement(goldengate.HeartbeatLagOptions{Source: source})
			}
			add(source.Owner()+"."+source.Name(), sql, args, err)
		}
	}
	if json.NewEncoder(os.Stdout).Encode(selections) != nil {
		fail()
	}
}

func fail() {
	_, _ = os.Stderr.WriteString("GoldenGate SQL selection or admitted catalog is invalid\n")
	os.Exit(1)
}
