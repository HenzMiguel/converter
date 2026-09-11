// Command converter is a small web app that converts images and videos using ffmpeg.
package main

import (
	"converter/internal"
	"log"
	"net/http"
	"os"
	"os/exec"

	gowiki "github.com/trietmn/go-wiki"
)

const (
	uploadDir = "uploads"
	outputDir = "output"
)

func main() {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Println("aviso: ffmpeg não encontrado no PATH; a conversão irá falhar até que ele seja instalado")
	}
	// Wikipedia requires a descriptive User-Agent identifying the client; anonymous ones get a 403.
	gowiki.SetUserAgent("converter-web-app/1.0 (https://github.com/; contact via repository issues)")
	gowiki.SetLanguage("pt")
	go internal.WikiWorker()

	for _, dir := range []string{uploadDir, outputDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("não foi possível criar diretório %s: %v", dir, err)
		}
	}

	internal.CleanupOutputDir()
	go internal.RunOutputJanitor()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", internal.HandleIndex)
	mux.HandleFunc("POST /convert", internal.HandleConvert)
	mux.HandleFunc("GET /download/", internal.HandleDownload)
	mux.HandleFunc("GET /formatinfo", internal.HandleFormatInfo)
	mux.Handle("GET /static/", http.FileServer(http.FS(internal.StaticFS)))

	addr := ":8082"
	if v := os.Getenv("PORT"); v != "" && internal.OnlyDigits(v) {
		addr = ":" + v
	}
	log.Printf("servidor ouvindo em %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
