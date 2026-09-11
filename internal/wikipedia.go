package internal

import (
	"log"

	gowiki "github.com/trietmn/go-wiki"
)

// go-wiki's internal cache is not safe for concurrent use, so requests to it
// are funneled through a single worker goroutine, which also owns the
// description cache below (no mutex needed since only it touches the map).
var wikiRequests = make(chan wikiRequest)

// wikiRequest asks the wiki worker goroutine for a format's description and
// delivers the result back on resp.
type wikiRequest struct {
	format string
	meta   formatMeta
	resp   chan formatInfo
}

// wikiWorker serializes all go-wiki calls onto a single goroutine and owns
// the in-memory description cache exclusively.
func WikiWorker() {
	cache := map[string]formatInfo{}
	for req := range wikiRequests {
		if info, ok := cache[req.format]; ok {
			req.resp <- info
			continue
		}

		extract, err := gowiki.Summary(req.meta.WikiTitle, 3, -1, true, true)
		if err != nil {
			log.Printf("erro ao buscar descrição de %s: %v", req.meta.WikiTitle, err)
			extract = "Descrição indisponível no momento."
		}

		info := formatInfo{Title: req.meta.WikiTitle, Extract: extract, URL: wikipediaURL(req.meta.WikiTitle), Category: req.meta.Category}
		cache[req.format] = info
		req.resp <- info
	}
}

func fetchWikiInfo(format string, meta formatMeta) formatInfo {
	resp := make(chan formatInfo)
	wikiRequests <- wikiRequest{format: format, meta: meta, resp: resp}
	return <-resp
}

type formatInfo struct {
	Title    string `json:"title"`
	Extract  string `json:"extract"`
	URL      string `json:"url"`
	Category string `json:"category"`
}

// formatMeta describes an allowed output format: its Wikipedia article and
// whether it applies to images or videos (used to filter the format select).
type formatMeta struct {
	WikiTitle string
	Category  string
}
