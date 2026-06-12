package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"
)

// POST /api/documents/{id}/ocr-all — OCR many pages with a worker pool,
// streaming each page's result over SSE as it completes. Events:
//
//	result      {page, task_type, markdown, elapsed_seconds}
//	page_error  {page, detail}
//	done        {ok, failed: [pages]}
//
// A page failure doesn't stop the run; the client retries failed pages by
// resubmitting them. Closing the connection cancels in-flight model calls.

type batchRequest struct {
	Pages          []int  `json:"pages"` // empty = all pages
	TaskType       string `json:"task_type"`
	FigureLanguage string `json:"figure_language"`
}

type pageOutcome struct {
	page    int
	md      string
	elapsed float64
	err     error
}

func handleOCRAll(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, 400, "Invalid JSON body: "+err.Error())
		return
	}
	if msg := validateOCRParams(&req.TaskType, &req.FigureLanguage); msg != "" {
		jsonError(w, 400, msg)
		return
	}
	doc, ok := store.Get(r.PathValue("id"))
	if !ok {
		jsonError(w, 404, "Document not found. It may have expired — upload it again.")
		return
	}

	pages := req.Pages
	if len(pages) == 0 {
		for p := 1; p <= doc.Pages; p++ {
			pages = append(pages, p)
		}
	}
	seen := make(map[int]bool, len(pages))
	uniq := pages[:0]
	for _, p := range pages {
		if p < 1 || p > doc.Pages {
			jsonError(w, 400, fmt.Sprintf("Page %d is out of range (1–%d).", p, doc.Pages))
			return
		}
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}
	pages = uniq

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, 500, "Streaming is not supported by this connection.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(200)
	flusher.Flush()

	ctx := r.Context()
	jobs := make(chan int)
	outcomes := make(chan pageOutcome)

	workers := cfg.OCRConcurrency
	if workers > len(pages) {
		workers = len(pages)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				started := time.Now()
				md, err := ocrPage(ctx, doc, p, req.TaskType, req.FigureLanguage)
				elapsed := float64(int(time.Since(started).Seconds()*100)) / 100
				select {
				case outcomes <- pageOutcome{p, md, elapsed, err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, p := range pages {
			select {
			case jobs <- p:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(outcomes)
	}()

	okCount := 0
	failed := []int{} // non-nil so "done" serializes failed as [] not null
	for out := range outcomes {
		if out.err != nil {
			if ctx.Err() != nil {
				continue // cancelled; drain without writing
			}
			// Full error (which may embed the endpoint URL) goes to the log only.
			log.Printf("OCR request failed for page %d of %s: %v", out.page, doc.Name, out.err)
			failed = append(failed, out.page)
			writeSSE(w, flusher, "page_error", map[string]any{
				"page": out.page, "detail": "OCR failed — check the server log.",
			})
			continue
		}
		okCount++
		writeSSE(w, flusher, "result", map[string]any{
			"page":            out.page,
			"task_type":       req.TaskType,
			"markdown":        out.md,
			"elapsed_seconds": out.elapsed,
		})
	}
	if ctx.Err() == nil {
		sort.Ints(failed)
		writeSSE(w, flusher, "done", map[string]any{"ok": okCount, "failed": failed})
	}
}

func writeSSE(w http.ResponseWriter, f http.Flusher, event string, v any) {
	data, _ := json.Marshal(v) // JSON escapes newlines, so data stays one line
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	f.Flush()
}
