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
	log.Println("Preparing prompt for AI with question:", question)
	prompt := fmt.Sprintf(`Vygeneruj tři špatné, ale věrohodné odpovědi k následující otázce. Neuváděj správnou odpověď. Odpovědi uveď v následujícím formátu, každou odpověď začni hvězdičkou (asterisk):

Otázka: %s
Správná odpověď: %s`, question, correctAnswer)

	payload := OpenRouterRequest{
		Model: "meta-llama/llama-4-scout:free",
		Messages: []Message{
			{Role: "system", Content: "Jsi asistent pro tvorbu kvízových otázek na českých středních a vysokých školách."},
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
		log.Println("Error making request to OpenRouter API:", err)
		return nil, err
	}
	defer resp.Body.Close()

	resBody, _ := ioutil.ReadAll(resp.Body)
	log.Println("Received response from OpenRouter API:", string(resBody))

	var parsed OpenRouterResponse
	err = json.Unmarshal(resBody, &parsed)
	if err != nil || len(parsed.Choices) == 0 {
		log.Println("Error parsing AI response or empty choices")
		return nil, fmt.Errorf("chyba při zpracování odpovědi AI")
	}

	responseText := parsed.Choices[0].Message.Content
	log.Println("AI response text:", responseText)

	// Split the response into lines and extract the wrong answers
	lines := strings.Split(responseText, "\n")
	var wrongAnswers []string
	for _, line := range lines {
		// Clean up the line by removing the numbering, asterisks, and extra spaces
		line = strings.TrimSpace(strings.TrimLeft(line, "-0123456789. *"))
		if line != "" && len(wrongAnswers) < 3 {
			wrongAnswers = append(wrongAnswers, line)
		}
	}

	log.Println("Generated wrong answers:", wrongAnswers)
	return wrongAnswers, nil
}

func convertExcelToText(file io.Reader) (string, error) {
	log.Println("Starting to read Excel file")
	f, err := excelize.OpenReader(file)
	if err != nil {
		log.Println("Error opening Excel file:", err)
		return "", err
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		log.Println("Error reading rows from Excel sheet:", err)
		return "", err
	}

	log.Println("Reading rows from Excel sheet:", len(rows))
	var result string
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		question := row[0]
		correct := row[1]

		log.Println("Processing question:", question, "with correct answer:", correct)
		wrongAnswers, err := getWrongAnswers(question, correct)
		if err != nil {
			log.Println("Error getting wrong answers:", err)
			continue
		}

		result += fmt.Sprintf(".%s\n", question)
		result += fmt.Sprintf("..%s\n", correct)
		for _, wrong := range wrongAnswers {
			result += fmt.Sprintf("...%s\n", wrong)
		}
	}
	log.Println("Finished processing Excel file")
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
		log.Println("Error processing file upload:", err)
		http.Error(w, "Chyba při nahrávání souboru", http.StatusBadRequest)
		return
	}
	defer file.Close()

	convertedText, err := convertExcelToText(file)
	if err != nil {
		log.Println("Error converting Excel to text:", err)
		http.Error(w, "Chyba při zpracování souboru: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Println("Returning converted text in response")
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

	log.Println("Server running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
