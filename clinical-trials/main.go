package main

import (
	"encoding/json"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]
	isJSON := false
	query := ""

	for i, arg := range args {
		if arg == "--json" {
			isJSON = true
		}
		if strings.HasPrefix(arg, "search") && i+1 < len(args) {
			query = args[i+1]
		}
		if strings.HasPrefix(arg, "recruiting") && i+1 < len(args) {
			query = args[i+1]
		}
	}

	result := map[string]interface{}{
		"status":        "ok",
		"query":         query,
		"total_results": 42,
		"message":       "Ez egy TESZT clinical-trials CLI!",
		"results": []map[string]interface{}{
			{"nct_id": "NCT12345678", "title": "Teszt vizsgálat 1", "phase": "Phase 3", "status": "RECRUITING"},
			{"nct_id": "NCT87654321", "title": "Teszt vizsgálat 2", "phase": "Phase 2", "status": "ACTIVE"},
		},
	}

	if isJSON {
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		os.Stdout.WriteString("Használd a --json flag-et!\n")
	}
}