// Command queries prints read-only selectors without opening a database.
package main

import (
	"fmt"
	"log"

	"github.com/marcuoli/go-oracle-info/backup"
	"github.com/marcuoli/go-oracle-info/objects"
)

func main() {
	text, args := objects.InventoryStatement("HR", "OE")
	fmt.Printf("%s\nBind owners: %v\n", text, args)
	jobs, err := backup.JobsSQL(10)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(jobs)
}
