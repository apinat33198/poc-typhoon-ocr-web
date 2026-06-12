// Typhoon OCR Web — Go backend.
//
// A self-hostable server for scb10x/typhoon-ocr. Renders PDF pages with
// poppler, builds the typhoon prompt, and calls any OpenAI-compatible
// endpoint (local vLLM or api.opentyphoon.ai). Stdlib only.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BaseURL        string
	APIKey         string
	Model          string
	Port           string
	WebDir         string
	MaxUploadMB    int64
	OCRConcurrency int
}

var (
	cfg   Config
	store *Store
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// loadDotEnv reads KEY=VALUE lines from .env if present; real env vars win.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && os.Getenv(strings.TrimSpace(k)) == "" {
			os.Setenv(strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"detail": msg})
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// resolveWebDir picks the frontend build directory: $WEB_DIR if set,
// otherwise the first candidate containing an index.html, checked relative
// to the working directory and to the binary's location. This lets the
// server be launched from the repo root, from server/, or anywhere else.
func resolveWebDir() string {
	if v := os.Getenv("WEB_DIR"); v != "" {
		return v
	}
	candidates := []string{"web/dist", "../web/dist", "dist"}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "../web/dist"),
			filepath.Join(dir, "web/dist"))
	}
	for _, c := range candidates {
		if st, err := os.Stat(filepath.Join(c, "index.html")); err == nil && !st.IsDir() {
			return c
		}
	}
	return "web/dist" // nothing found; startup warning points the user at the build step
}

func main() {
	loadDotEnv(".env")
	loadDotEnv("../.env") // also works when launched from server/
	maxMB, _ := strconv.ParseInt(envOr("MAX_UPLOAD_MB", "50"), 10, 64)
	conc, _ := strconv.Atoi(envOr("OCR_CONCURRENCY", "4"))
	if conc < 1 {
		conc = 1
	}
	cfg = Config{
		BaseURL:        strings.TrimRight(envOr("TYPHOON_BASE_URL", "http://localhost:8101/v1"), "/"),
		APIKey:         envOr("TYPHOON_API_KEY", "no-key"),
		Model:          envOr("TYPHOON_OCR_MODEL", "typhoon-ocr"),
		Port:           envOr("PORT", "7870"),
		WebDir:         resolveWebDir(),
		MaxUploadMB:    maxMB,
		OCRConcurrency: conc,
	}
	if _, err := os.Stat(filepath.Join(cfg.WebDir, "index.html")); err != nil {
		log.Printf("WARNING: frontend build not found at %q — run `npm install && npm run build` in web/, or set WEB_DIR", cfg.WebDir)
	}
	store = NewStore(time.Hour)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", handleConfig)
	mux.HandleFunc("POST /api/documents", handleUpload)
	mux.HandleFunc("GET /api/documents/{id}/pages/{n}", handlePreview)
	mux.HandleFunc("POST /api/documents/{id}/ocr", handleOCR)
	mux.HandleFunc("POST /api/documents/{id}/ocr-all", handleOCRAll)
	mux.Handle("/", http.FileServer(http.Dir(cfg.WebDir)))

	log.Printf("typhoon-ocr-web listening on :%s  (model %s @ %s)", cfg.Port, cfg.Model, cfg.BaseURL)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		log.Fatal(err)
	}
}

// The base URL is intentionally not exposed: it can reveal internal hosts.
func handleConfig(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]string{"model": cfg.Model})
}

var allowedExt = map[string]bool{".pdf": true, ".png": true, ".jpg": true, ".jpeg": true}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxUploadMB<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, 400, fmt.Sprintf("Could not read the upload (max %d MB): %v", cfg.MaxUploadMB, err))
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExt[ext] {
		jsonError(w, 400, fmt.Sprintf("Unsupported file type %q. Use PDF, PNG, or JPG.", ext))
		return
	}

	tmp, err := os.CreateTemp("", "typhoon-*"+ext)
	if err != nil {
		jsonError(w, 500, "Could not create a temporary file: "+err.Error())
		return
	}
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		jsonError(w, 500, "Could not save the upload: "+err.Error())
		return
	}
	tmp.Close()

	isPDF := ext == ".pdf"
	pages := 1
	if isPDF {
		pages, err = pdfPageCount(tmp.Name())
		if err != nil {
			os.Remove(tmp.Name())
			jsonError(w, 400, "Could not read PDF: "+err.Error())
			return
		}
	}

	doc := store.Add(tmp.Name(), header.Filename, pages, isPDF)
	jsonOK(w, map[string]any{
		"doc_id": doc.ID, "filename": doc.Name, "pages": doc.Pages, "is_pdf": doc.IsPDF,
	})
}

func docAndPage(w http.ResponseWriter, r *http.Request, page int) (*Doc, bool) {
	doc, ok := store.Get(r.PathValue("id"))
	if !ok {
		jsonError(w, 404, "Document not found. It may have expired — upload it again.")
		return nil, false
	}
	if page < 1 || page > doc.Pages {
		jsonError(w, 400, fmt.Sprintf("Page %d is out of range (1–%d).", page, doc.Pages))
		return nil, false
	}
	return doc, true
}

func handlePreview(w http.ResponseWriter, r *http.Request) {
	page, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		jsonError(w, 400, "Page must be a number.")
		return
	}
	doc, ok := docAndPage(w, r, page)
	if !ok {
		return
	}
	if !doc.IsPDF {
		http.ServeFile(w, r, doc.Path)
		return
	}
	width := 1100
	if v, err := strconv.Atoi(r.URL.Query().Get("w")); err == nil {
		width = clamp(v, 200, 3000)
	}
	png, err := renderPDFPagePNG(doc.Path, page, width)
	if err != nil {
		jsonError(w, 500, "Could not render page preview: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

type ocrRequest struct {
	Page           int    `json:"page"`
	TaskType       string `json:"task_type"`
	FigureLanguage string `json:"figure_language"`
}

// validateOCRParams fills in defaults and returns an error message for
// invalid mode combinations ("" when valid). Shared by the single-page and
// batch endpoints.
func validateOCRParams(taskType, figureLanguage *string) string {
	if *taskType == "" {
		*taskType = "v1.5"
	}
	if *figureLanguage == "" {
		*figureLanguage = "Thai"
	}
	if *taskType != "default" && *taskType != "structure" && *taskType != "v1.5" {
		return "task_type must be 'default', 'structure', or 'v1.5'."
	}
	if *figureLanguage != "Thai" && *figureLanguage != "English" {
		return "figure_language must be 'Thai' or 'English'."
	}
	if strings.Contains(cfg.Model, "typhoon-ocr-preview") && *taskType == "v1.5" {
		return "The hosted typhoon-ocr-preview model only supports 'default' and 'structure' modes."
	}
	return ""
}

func handleOCR(w http.ResponseWriter, r *http.Request) {
	var req ocrRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, 400, "Invalid JSON body: "+err.Error())
		return
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if msg := validateOCRParams(&req.TaskType, &req.FigureLanguage); msg != "" {
		jsonError(w, 400, msg)
		return
	}
	doc, ok := docAndPage(w, r, req.Page)
	if !ok {
		return
	}

	messages, err := buildOCRMessages(doc, req.Page, req.TaskType, req.FigureLanguage)
	if err != nil {
		jsonError(w, 500, "Could not prepare the page for OCR: "+err.Error())
		return
	}

	started := time.Now()
	markdown, err := callModel(r.Context(), messages, req.TaskType)
	if err != nil {
		// Full error (which may embed the endpoint URL) goes to the log only.
		log.Printf("OCR request failed for page %d of %s: %v", req.Page, doc.Name, err)
		jsonError(w, 502, "OCR model request failed — check the server log and that the model endpoint is up.")
		return
	}

	jsonOK(w, map[string]any{
		"page":            req.Page,
		"task_type":       req.TaskType,
		"markdown":        markdown,
		"elapsed_seconds": float64(int(time.Since(started).Seconds()*100)) / 100,
	})
}
