package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/boangs/remarkable-space/internal/obsidian"
)

func main() {
	source := flag.String("source", "", "reMarkable xochitl directory (local backup or mounted device)")
	vault := flag.String("vault", "", "Obsidian vault directory")
	flag.Parse()
	if *source == "" || *vault == "" {
		flag.Usage()
		return
	}
	result, err := (obsidian.Exporter{Source: *source, Vault: *vault}).Run()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("exported %d document(s), %d page(s), %d page(s) with embedded text\n",
		result.Documents, result.Pages, result.TypedText)
}
