package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./medical.db")
	if err != nil {
		log.Fatal(err)
	}

	tables := []string{
		`CREATE TABLE IF NOT EXISTS reading_list (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			doi TEXT UNIQUE,
			title TEXT,
			journal TEXT,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS search_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			query TEXT,
			command TEXT,
			result TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS cache (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT UNIQUE,
			value TEXT,
			expires_at DATETIME
		)`,
	}

	for _, sql := range tables {
		if _, err := db.Exec(sql); err != nil {
			log.Printf("⚠️ Tábla hiba: %v", err)
		}
	}
	log.Println("✅ Adatbázis készen!")
}

func runCLI(command string, args ...string) (map[string]interface{}, error) {
	cliMap := map[string]string{
		"clinical-trials":      "../clinical-trials/clinical-trials.exe",
		"scientific-consensus": "../scientific-consensus/scientific-consensus.exe",
		"thelancet":            "../thelancet/thelancet.exe",
		"nejm":                 "../nejm/nejm.exe",
	}

	bin, ok := cliMap[command]
	if !ok {
		return nil, fmt.Errorf("ismeretlen CLI: %s", command)
	}

	if _, err := os.Stat(bin); os.IsNotExist(err) {
		bin = "./" + command + ".exe"
		if _, err := os.Stat(bin); os.IsNotExist(err) {
			return nil, fmt.Errorf("CLI nem található: %s", bin)
		}
	}

	cmdArgs := append([]string{"--json"}, args...)
	cmd := exec.Command(bin, cmdArgs...)

	log.Printf("🔧 Futtatás: %s %v", bin, cmdArgs)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var output []byte
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		output = append(output, scanner.Bytes()...)
	}

	errOutput, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("CLI hiba: %v - %s", err, string(errOutput))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("JSON parse hiba: %v - %s", err, string(output))
	}

	return result, nil
}

func handleConsensus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Csak POST", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Hibás kérés", http.StatusBadRequest)
		return
	}
	if req.Limit == 0 {
		req.Limit = 20
	}

	result, err := runCLI("scientific-consensus", "consensus", req.Query, "--limit", fmt.Sprintf("%d", req.Limit))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Hiányzó 'q' paraméter", http.StatusBadRequest)
		return
	}

	result, err := runCLI("clinical-trials", "search", query, "--limit", "20")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleReadingList(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		rows, err := db.Query("SELECT id, doi, title, journal, added_at FROM reading_list ORDER BY added_at DESC")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var items []map[string]interface{}
		for rows.Next() {
			var id int
			var doi, title, journal, addedAt string
			rows.Scan(&id, &doi, &title, &journal, &addedAt)
			items = append(items, map[string]interface{}{
				"id":       id,
				"doi":      doi,
				"title":    title,
				"journal":  journal,
				"added_at": addedAt,
			})
		}
		json.NewEncoder(w).Encode(items)

	case "POST":
		var req struct {
			DOI     string `json:"doi"`
			Title   string `json:"title"`
			Journal string `json:"journal"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Hibás kérés", http.StatusBadRequest)
			return
		}

		_, err := db.Exec("INSERT OR IGNORE INTO reading_list (doi, title, journal) VALUES (?, ?, ?)",
			req.DOI, req.Title, req.Journal)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case "DELETE":
		var req struct {
			DOI string `json:"doi"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Hibás kérés", http.StatusBadRequest)
			return
		}
		_, err := db.Exec("DELETE FROM reading_list WHERE doi = ?", req.DOI)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func handleRankAuthors(w http.ResponseWriter, r *http.Request) {
	institution := r.URL.Query().Get("institution")
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "10"
	}

	args := []string{"rank-authors"}
	if institution != "" {
		args = append(args, "--institution", institution)
	}
	args = append(args, "--limit", limit)

	result, err := runCLI("thelancet", args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"version":   "1.0.0",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func main() {
	initDB()

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	http.HandleFunc("/api/consensus", corsMiddleware(handleConsensus))
	http.HandleFunc("/api/search", corsMiddleware(handleSearch))
	http.HandleFunc("/api/reading-list", corsMiddleware(handleReadingList))
	http.HandleFunc("/api/rank-authors", corsMiddleware(handleRankAuthors))
	http.HandleFunc("/api/health", corsMiddleware(handleHealth))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "./static/index.html")
			return
		}
		http.NotFound(w, r)
	})

	port := "8080"
	log.Printf("🚀 Szerver indul: http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}