package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/xuri/excelize/v2"
)

const openRouterAPIKey = "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"
const openRouterEndpoint = "https://openrouter.ai/api/v1/chat/completions"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenRouterRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type OpenRouterResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

func getWrongAnswers(question, correctAnswer string) ([]string, error) {
	prompt := fmt.Sprintf(`Vygeneruj tři špatné, ale věrohodné odpovědi k následující otázce. Neuváděj správnou odpověď. Vrať pouze seznam tří špatných odpovědí.

Otázka: %s
Správná odpověď: %s`, question, correctAnswer)

	payload := OpenRouterRequest{
		Model: "meta-llama/llama-3.2-1b-instruct:free",
		Messages: []Message{
			{Role: "system", Content: "Jsi asistent pro tvorbu kvízových otázek na českých školách."},
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", openRouterEndpoint, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+openRouterAPIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "*")
	req.Header.Set("X-Title", "Excel Špatné Odpovědi")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	resBody, _ := ioutil.ReadAll(resp.Body)

	var parsed OpenRouterResponse
	err = json.Unmarshal(resBody, &parsed)
	if err != nil || len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("chyba při zpracování odpovědi AI")
	}

	responseText := parsed.Choices[0].Message.Content
	lines := strings.Split(responseText, "\n")

	var wrongAnswers []string
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimLeft(line, "-0123456789. "))
		if line != "" {
			wrongAnswers = append(wrongAnswers, line)
		}
		if len(wrongAnswers) == 3 {
			break
		}
	}

	return wrongAnswers, nil
}

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
		if len(row) < 2 {
			continue
		}
		question := row[0]
		correct := row[1]

		wrongAnswers, err := getWrongAnswers(question, correct)
		if err != nil {
			log.Println("Chyba při získávání špatných odpovědí:", err)
			continue
		}

		result += fmt.Sprintf(".%s\n", question)
		result += fmt.Sprintf("..%s\n", correct)
		for _, wrong := range wrongAnswers {
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
		http.Error(w, "Chyba při nahrávání souboru", http.StatusBadRequest)
		return
	}
	defer file.Close()

	convertedText, err := convertExcelToText(file)
	if err != nil {
		http.Error(w, "Chyba při zpracování souboru: "+err.Error(), http.StatusInternalServerError)
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

	fmt.Println("Server běží na portu", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
