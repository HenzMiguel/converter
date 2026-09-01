// Command converter is a small web app that converts images and videos using ffmpeg.
package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	gowiki "github.com/trietmn/go-wiki"
)

const (
	uploadDir     = "uploads"
	outputDir     = "output"
	maxUploadSize = 500 << 20 // 500 MB
	ffmpegTimeout = 10 * time.Minute

	// converted files are removed this long after being written to save disk space.
	outputTTL       = 15 * time.Minute
	cleanupInterval = 5 * time.Minute
)

//go:embed templates/index.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

// category labels used to filter the output format select on the client.
const (
	categoryImage = "imagem"
	categoryVideo = "video"
)

// formatMeta describes an allowed output format: its Wikipedia article and
// whether it applies to images or videos (used to filter the format select).
type formatMeta struct {
	WikiTitle string
	Category  string
}

var allowedFormats = map[string]formatMeta{
	// images
	"jpg": {"JPEG", categoryImage}, "jpeg": {"JPEG", categoryImage},
	"png": {"Portable Network Graphics", categoryImage}, "webp": {"WebP", categoryImage},
	"gif": {"GIF", categoryImage}, "bmp": {"BMP", categoryImage}, "tiff": {"TIFF", categoryImage},
	// videos / audio-visual
	"mp4": {"MPEG-4", categoryVideo}, "avi": {"Audio Video Interleave", categoryVideo},
	"mov": {"QuickTime File Format", categoryVideo}, "mkv": {"Matroska", categoryVideo},
	"webm": {"WebM", categoryVideo}, "flv": {"Flash Video", categoryVideo},
}

var indexTmpl = template.Must(template.ParseFS(templatesFS, "templates/index.html"))

// formatOption is a single entry rendered in the format <select> elements.
type formatOption struct {
	Value    string
	Category string
}

type pageData struct {
	Formats     []formatOption
	DefaultFrom string
	DefaultTo   string
}

type convertResult struct {
	Error        string `json:"error,omitempty"`
	DownloadName string `json:"downloadName,omitempty"`
}

type formatInfo struct {
	Title    string `json:"title"`
	Extract  string `json:"extract"`
	URL      string `json:"url"`
	Category string `json:"category"`
}

var (
	wikiCacheMu sync.RWMutex
	wikiCache   = map[string]formatInfo{}

	// go-wiki's internal cache is not safe for concurrent use, so requests to it
	// are funneled through a single worker goroutine via this channel.
	wikiRequests = make(chan wikiRequest)
)

// wikiRequest asks the wiki worker goroutine for a title's summary and
// delivers the result back on resp.
type wikiRequest struct {
	title string
	resp  chan wikiSummaryResult
}

type wikiSummaryResult struct {
	extract string
	err     error
}

// wikiWorker serializes all go-wiki calls onto a single goroutine.
func wikiWorker() {
	for req := range wikiRequests {
		extract, err := gowiki.Summary(req.title, 3, -1, true, true)
		req.resp <- wikiSummaryResult{extract: extract, err: err}
	}
}

func fetchWikiSummary(title string) (string, error) {
	resp := make(chan wikiSummaryResult)
	wikiRequests <- wikiRequest{title: title, resp: resp}
	result := <-resp
	return result.extract, result.err
}

func main() {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Println("aviso: ffmpeg não encontrado no PATH; a conversão irá falhar até que ele seja instalado")
	}

	// Wikipedia requires a descriptive User-Agent identifying the client; anonymous ones get a 403.
	gowiki.SetUserAgent("converter-web-app/1.0 (https://github.com/; contact via repository issues)")
	gowiki.SetLanguage("pt")
	go wikiWorker()

	for _, dir := range []string{uploadDir, outputDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("não foi possível criar diretório %s: %v", dir, err)
		}
	}

	cleanupOutputDir()
	go runOutputJanitor()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/convert", handleConvert)
	mux.HandleFunc("/download/", handleDownload)
	mux.HandleFunc("/formatinfo", handleFormatInfo)
	mux.Handle("/static/", http.FileServer(http.FS(staticFS)))

	addr := ":8082"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}
	log.Printf("servidor ouvindo em %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	formats := sortedFormatOptions()
	from, to := formats[0].Value, formats[0].Value
	if len(formats) > 1 {
		to = formats[1].Value
	}
	renderIndex(w, pageData{Formats: formats, DefaultFrom: from, DefaultTo: to})
}

func handleConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respondConvertError(w, http.StatusBadRequest, "arquivo muito grande ou formulário inválido")
		return
	}

	targetFormat := strings.ToLower(strings.TrimSpace(r.FormValue("format")))
	if _, ok := allowedFormats[targetFormat]; !ok {
		respondConvertError(w, http.StatusBadRequest, "formato de saída não suportado")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondConvertError(w, http.StatusBadRequest, "nenhum arquivo enviado")
		return
	}
	defer file.Close()

	id, err := randomID()
	if err != nil {
		respondConvertError(w, http.StatusInternalServerError, "erro interno ao gerar identificador")
		return
	}

	srcExt := strings.ToLower(filepath.Ext(header.Filename))
	inputPath := filepath.Join(uploadDir, id+srcExt)

	dst, err := os.Create(inputPath)
	if err != nil {
		respondConvertError(w, http.StatusInternalServerError, "erro ao salvar arquivo enviado")
		return
	}
	if _, err := dst.ReadFrom(file); err != nil {
		dst.Close()
		os.Remove(inputPath)
		respondConvertError(w, http.StatusInternalServerError, "erro ao gravar arquivo enviado")
		return
	}
	dst.Close()
	defer os.Remove(inputPath)

	outputName := id + "." + targetFormat
	outputPath := filepath.Join(outputDir, outputName)

	ctx, cancel := context.WithTimeout(r.Context(), ffmpegTimeout)
	defer cancel()

	// -y overwrites output if it exists; input/output paths are generated server-side, not user-controlled.
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inputPath, outputPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("falha na conversão ffmpeg: %v\n%s", err, out)
		respondConvertError(w, http.StatusUnprocessableEntity, "falha ao converter o arquivo; verifique se o formato de entrada é válido")
		return
	}

	writeJSON(w, convertResult{DownloadName: outputName})
}

// respondConvertError writes a JSON error response for the /convert endpoint with the given status.
func respondConvertError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(convertResult{Error: message}); err != nil {
		log.Printf("erro ao codificar JSON: %v", err)
	}
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path[len("/download/"):])
	if name == "" || name == "." || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}

	path := filepath.Join(outputDir, name)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	http.ServeFile(w, r, path)
}

// handleFormatInfo returns a short history/description of a file format, sourced
// from the Portuguese Wikipedia (via go-wiki) and cached in memory.
func handleFormatInfo(w http.ResponseWriter, r *http.Request) {
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	meta, ok := allowedFormats[format]
	if !ok {
		http.Error(w, "formato desconhecido", http.StatusBadRequest)
		return
	}

	wikiCacheMu.RLock()
	cached, found := wikiCache[format]
	wikiCacheMu.RUnlock()
	if found {
		writeJSON(w, cached)
		return
	}

	extract, err := fetchWikiSummary(meta.WikiTitle)
	if err != nil {
		log.Printf("erro ao buscar descrição de %s: %v", meta.WikiTitle, err)
		extract = "Descrição indisponível no momento."
	}

	info := formatInfo{Title: meta.WikiTitle, Extract: extract, URL: wikipediaURL(meta.WikiTitle), Category: meta.Category}
	wikiCacheMu.Lock()
	wikiCache[format] = info
	wikiCacheMu.Unlock()
	writeJSON(w, info)
}

func wikipediaURL(title string) string {
	return "https://pt.wikipedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(title, " ", "_"))
}

// runOutputJanitor periodically removes converted files older than outputTTL.
func runOutputJanitor() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		cleanupOutputDir()
	}
}

func cleanupOutputDir() {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		log.Printf("erro ao listar %s: %v", outputDir, err)
		return
	}

	cutoff := time.Now().Add(-outputTTL)
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		path := filepath.Join(outputDir, entry.Name())
		if err := os.Remove(path); err != nil {
			log.Printf("erro ao remover %s: %v", path, err)
		}
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
