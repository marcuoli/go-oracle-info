// Command hidden-context-selectors prints reviewed SQL selection metadata as JSON.
// It opens no database connection and serializes no query results.
package main

import (
	"encoding/json"
	"flag"
	"github.com/marcuoli/go-oracle-info/parameters"
	"os"
)

func main() {
	major := flag.Int("oracle-major", 19, "observed Oracle major version")
	flag.Parse()
	type choice struct {
		Name string `json:"name"`
		SQL  string `json:"sql"`
		Args []any  `json:"args"`
	}
	result := []choice{}
	for _, item := range []struct {
		name   string
		filter parameters.HiddenContextFilter
	}{{"all", parameters.HiddenContextFilter{}}, {"nondefault", parameters.HiddenContextFilter{NonDefaultOnly: true}}, {"exact", parameters.HiddenContextFilter{Names: []string{"compatible"}}}} {
		sql, args, err := parameters.HiddenContextStatement(*major, item.filter)
		if err != nil {
			os.Stderr.WriteString("hidden context selection failed\n")
			os.Exit(1)
		}
		result = append(result, choice{Name: item.name, SQL: sql, Args: args})
	}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		os.Exit(1)
	}
}
