// Command hidden-catalog-selectors emits SQL selection metadata, not query data.
// It lets a caller inspect the version-admitted catalog and its bound filters.
package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/marcuoli/go-oracle-info/parameters"
)

func main() {
	major := flag.Int("oracle-major", 19, "observed Oracle major version")
	flag.Parse()
	type selection struct {
		Name string `json:"name"`
		SQL  string `json:"sql"`
		Args []any  `json:"args"`
	}
	var selections []selection
	for _, item := range []struct {
		name   string
		filter parameters.HiddenCatalogFilter
	}{
		{"all", parameters.HiddenCatalogFilter{}},
		{"waiters", parameters.HiddenCatalogFilter{NameContains: "waiters"}},
		{"exact", parameters.HiddenCatalogFilter{Names: []string{"_optimizer_use_feedback"}}},
	} {
		sql, args, err := parameters.HiddenCatalogStatement(*major, item.filter)
		if err != nil {
			_, _ = os.Stderr.WriteString("invalid parameter selection\n")
			os.Exit(1)
		}
		selections = append(selections, selection{Name: item.name, SQL: sql, Args: args})
	}
	if err := json.NewEncoder(os.Stdout).Encode(selections); err != nil {
		os.Exit(1)
	}
}
