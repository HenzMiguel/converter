package internal

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func CleanupOutputDir() {
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

// runOutputJanitor periodically removes converted files older than outputTTL.
func RunOutputJanitor() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		CleanupOutputDir()
	}
}
