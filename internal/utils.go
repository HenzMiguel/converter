package internal

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

func sortedFormatOptions() []formatOption {
	options := make([]formatOption, 0, len(allowedFormats))
	for ext, meta := range allowedFormats {
		options = append(options, formatOption{Value: ext, Category: meta.Category})
	}
	sort.Slice(options, func(i, j int) bool { return options[i].Value < options[j].Value })
	return options
}

// randomID returns a filename-safe unique ID derived from 16 cryptographically
// secure random bytes (128 bits), which makes collisions negligibly unlikely.
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func wikipediaURL(title string) string {
	return "https://pt.wikipedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(title, " ", "_"))
}

// respondConvertError writes a JSON error response for the /convert endpoint with the given status.
func respondConvertError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(convertResult{Error: message}); err != nil {
		log.Printf("erro ao codificar JSON: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("erro ao codificar JSON: %v", err)
	}
}

func renderIndex(w http.ResponseWriter, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTmpl.Execute(w, data); err != nil {
		log.Printf("erro ao renderizar template: %v", err)
	}
}

func OnlyDigits(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
