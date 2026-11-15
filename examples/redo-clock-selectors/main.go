// Command redo-clock-selectors prints source-clock SQL metadata as JSON.
// It opens no database connection and serializes no query-result data.
package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/marcuoli/go-oracle-info/redo"
)

func main() {
	major := flag.Int("oracle-major", 19, "observed Oracle major version")
	flag.Parse()
	statement, err := redo.ClockStatement(*major)
	if err != nil {
		os.Stderr.WriteString("clock selection failed\n")
		os.Exit(1)
	}
	selected := struct {
		OracleMajor int    `json:"oracle_major"`
		SQL         string `json:"sql"`
	}{*major, statement}
	if json.NewEncoder(os.Stdout).Encode(selected) != nil {
		os.Exit(1)
	}
}
