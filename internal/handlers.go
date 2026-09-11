package internal

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"
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
var StaticFS embed.FS

// category labels used to filter the output format select on the client.
const (
	categoryImage = "imagem"
	categoryVideo = "video"
)

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

var indexTmpl = template.Must(template.ParseFS(templatesFS, "templates/index.html"))

func HandleIndex(w http.ResponseWriter, r *http.Request) {
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

func HandleConvert(w http.ResponseWriter, r *http.Request) {

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

// HandleFormatInfo returns a short history/description of a file format, sourced
// from the Portuguese Wikipedia (via go-wiki) and cached in memory.
func HandleFormatInfo(w http.ResponseWriter, r *http.Request) {
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	meta, ok := allowedFormats[format]
	if !ok {
		http.Error(w, "formato desconhecido", http.StatusBadRequest)
		return
	}

	writeJSON(w, fetchWikiInfo(format, meta))
}

func HandleDownload(w http.ResponseWriter, r *http.Request) {
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
