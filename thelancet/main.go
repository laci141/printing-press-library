package main

import (
	"encoding/json"
	"os"
)

func main() {
	args := os.Args[1:]
	isJSON := false
	institution := ""

	for i, arg := range args {
		if arg == "--json" {
			isJSON = true
		}
		if arg == "--institution" && i+1 < len(args) {
			institution = args[i+1]
		}
	}

	result := map[string]interface{}{
		"status":      "ok",
		"institution": institution,
		"message":     "Ez egy TESZT thelancet CLI!",
		"authors": []map[string]interface{}{
			{"name": "Teszt Szerző 1", "works": 15, "citations": 1234},
			{"name": "Teszt Szerző 2", "works": 10, "citations": 567},
		},
	}

	if isJSON {
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		os.Stdout.WriteString("Használd a --json flag-et!\n")
	}
}