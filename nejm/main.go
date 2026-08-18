package main

import (
	"encoding/json"
	"os"
)

func main() {
	args := os.Args[1:]
	isJSON := false
	doi := ""

	for i, arg := range args {
		if arg == "--json" {
			isJSON = true
		}
		if i+1 < len(args) && arg == "article" {
			doi = args[i+1]
		}
	}

	result := map[string]interface{}{
		"status":  "ok",
		"doi":     doi,
		"message": "Ez egy TESZT nejm CLI!",
		"article": map[string]interface{}{
			"title":     "Teszt NEJM cikk",
			"journal":   "N Engl J Med",
			"year":      2024,
			"citations": 89,
		},
	}

	if isJSON {
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		os.Stdout.WriteString("Használd a --json flag-et!\n")
	}
}