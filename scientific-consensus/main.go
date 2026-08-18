package main

import (
	"encoding/json"
	"os"
)

func main() {
	args := os.Args[1:]
	isJSON := false
	query := ""

	for i, arg := range args {
		if arg == "--json" {
			isJSON = true
		}
		if i+1 < len(args) && (arg == "consensus" || arg == "search") {
			query = args[i+1]
		}
	}

	result := map[string]interface{}{
		"status":           "ok",
		"query":            query,
		"consensus_score":  0.85,
		"verdict":          "evidence-supports",
		"confidence":       0.92,
		"supporting":       32,
		"refuting":         3,
		"study_count":      35,
		"total_citations":  124567,
		"message":          "Ez egy TESZT válasz! A valós CLI-hez fordítsd le a forráskódot.",
		"results": []map[string]interface{}{
			{"title": "Teszt tanulmány 1", "year": 2024, "citations": 150},
			{"title": "Teszt tanulmány 2", "year": 2023, "citations": 89},
			{"title": "Teszt tanulmány 3", "year": 2022, "citations": 45},
		},
	}

	if isJSON {
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		os.Stdout.WriteString("Használd a --json flag-et!\n")
	}
}