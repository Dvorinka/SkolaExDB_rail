package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/xuri/excelize/v2"
)

func convertExcelToText(file io.Reader) (string, error) {
	f, err := excelize.OpenReader(file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return "", err
	}

	var result string
	for _, row := range rows {
		if len(row) < 3 {
			continue
		}
		result += fmt.Sprintf(".%s\n", row[0])
		result += fmt.Sprintf("..%s\n", row[1])
		for _, wrong := range row[2:] {
			result += fmt.Sprintf("...%s\n", wrong)
		}
	}
	return result, nil
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")

	// Handle preflight (OPTIONS)
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parse file
	r.ParseMultipartForm(10 << 20) // 10MB

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "File upload error", http.StatusBadRequest)
		return
	}
	defer file.Close()

	convertedText, err := convertExcelToText(file)
	if err != nil {
		http.Error(w, "Failed to process file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"result": convertedText,
	})
}

func main() {
	http.HandleFunc("/convert", uploadHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Server running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
