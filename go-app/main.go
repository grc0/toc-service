package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────
// Configuration
// ──────────────────────────────────────────────

var (
	maxUploadMB              = envInt("MAX_UPLOAD_MB", 50)
	maxUploadBytes           = int64(maxUploadMB) * 1024 * 1024
	maxJSONMB                = envInt("MAX_JSON_MB", 2)
	maxJSONBytes             = int64(maxJSONMB) * 1024 * 1024
	maxFiles                 = envInt("MAX_FILES", 100)
	typstVersionTimeout      = time.Duration(envInt("TYPST_VERSION_TIMEOUT_SECONDS", 2)) * time.Second
	healthCheckTimeout       = time.Duration(envInt("HEALTHCHECK_TIMEOUT_SECONDS", 3)) * time.Second
)

// ──────────────────────────────────────────────
// Middleware
// ──────────────────────────────────────────────

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", requestID)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "request_id", requestID)
		next.ServeHTTP(w, r)
	})
}

// ──────────────────────────────────────────────
// Response helpers
// ──────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	if status >= 500 {
		writeJSON(w, status, map[string]string{"error": message})
	} else {
		writeJSON(w, status, map[string]string{"detail": message})
	}
}

func handleTocError(w http.ResponseWriter, err error) {
	var noPdfs *NoPdfsFoundError
	var invalidJSON *InvalidJsonError
	var typstErr *TypstCompilationError
	var tocErr *TocError

	switch {
	case errors.As(err, &noPdfs):
		writeError(w, http.StatusBadRequest, noPdfs.Message)
	case errors.As(err, &invalidJSON):
		writeError(w, http.StatusBadRequest, invalidJSON.Message)
	case errors.As(err, &typstErr):
		writeError(w, http.StatusInternalServerError, typstErr.Message)
	case errors.As(err, &tocErr):
		writeError(w, http.StatusInternalServerError, tocErr.Message)
	default:
		slog.Error("unexpected error", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// ──────────────────────────────────────────────
// Upload helpers
// ──────────────────────────────────────────────

func saveUploadFile(file multipart.File, header *multipart.FileHeader, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	written, err := io.Copy(out, io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		os.Remove(dest)
		return err
	}
	if written > maxUploadBytes {
		os.Remove(dest)
		return fmt.Errorf("Datei zu groß (max %d MB)", maxUploadMB)
	}
	return nil
}

func ptrStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ──────────────────────────────────────────────
// Handlers
// ──────────────────────────────────────────────

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join("static", "index.html"))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	typstVersion := "nicht installiert"
	if output, err := runCommandWithTimeout(typstVersionTimeout, "typst", "--version"); err == nil {
		typstVersion = strings.TrimSpace(string(output))
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":        "ok",
		"typst_version": typstVersion,
	})
}

func handleFonts(w http.ResponseWriter, r *http.Request) {
	fonts := make([]string, 0, len(AllowedFonts))
	for k := range AllowedFonts {
		fonts = append(fonts, k)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"fonts":   fonts,
		"default": DefaultFont,
	})
}

func handleColors(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"colors":  LineColors,
		"default": DefaultLineColor,
	})
}

func handleBgColors(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"colors":  BgColors,
		"default": DefaultBgColor,
	})
}

func handleFontColors(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"colors":  FontColors,
		"default": DefaultFontColor,
	})
}

func handleUsagePage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("static", "usage.html"))
}

func handleHowToPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("static", "howto.html"))
}

func handleLocalPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("static", "local.html"))
}

func handleUsageAPI(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n >= 1 && n <= 500 {
			limit = n
		}
	}
	stats, err := getStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	recent, err := getRecentCalls(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"stats":  stats,
		"recent": recent,
	})
}

func handleGenerateFolder(w http.ResponseWriter, r *http.Request) {
	tStart := time.Now()

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "Ungültige Anfrage.")
		return
	}

	title := r.FormValue("title")
	subtitle := r.FormValue("subtitle")
	font := r.FormValue("font")
	dotLeaders := strings.ToLower(r.FormValue("dot_leaders")) != "false"
	showLines := strings.ToLower(r.FormValue("show_lines")) != "false"
	lineColor := r.FormValue("line_color")
	bgColor := r.FormValue("bg_color")
	fontColor := r.FormValue("font_color")
	mergeOnly := strings.ToLower(r.FormValue("merge_only")) == "true"
	lang := r.FormValue("lang")
	if lang == "" {
		lang = "de"
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "Keine Dateien hochgeladen.")
		logFolderCall(tStart, 0, title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, mergeOnly, lang, "error", "Keine Dateien hochgeladen.")
		return
	}

	if len(files) > maxFiles {
		msg := fmt.Sprintf("Zu viele Dateien (%d). Maximal %d erlaubt.", len(files), maxFiles)
		writeError(w, http.StatusBadRequest, msg)
		logFolderCall(tStart, len(files), title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, mergeOnly, lang, "error", msg)
		return
	}

	// Validate all files are PDFs
	for _, fh := range files {
		if !strings.HasSuffix(strings.ToLower(fh.Filename), ".pdf") {
			msg := fmt.Sprintf("'%s' ist keine PDF-Datei.", fh.Filename)
			writeError(w, http.StatusBadRequest, msg)
			logFolderCall(tStart, len(files), title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, mergeOnly, lang, "error", msg)
			return
		}
	}

	// Save uploads to temp directory
	uploadDir, err := os.MkdirTemp("", "upload-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Interner Fehler.")
		return
	}
	defer os.RemoveAll(uploadDir)

	titles := r.MultipartForm.Value["titles"]
	pdfFiles := make([]PDFFile, 0, len(files))
	for i, fh := range files {
		src, err := fh.Open()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Fehler beim Lesen der Datei.")
			logFolderCall(tStart, len(files), title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, mergeOnly, lang, "error", err.Error())
			return
		}
		safeName := filepath.Base(fh.Filename)
		destPath := filepath.Join(uploadDir, safeName)
		if err := saveUploadFile(src, fh, destPath); err != nil {
			src.Close()
			writeError(w, http.StatusRequestEntityTooLarge, err.Error())
			logFolderCall(tStart, len(files), title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, mergeOnly, lang, "error", err.Error())
			return
		}
		src.Close()
		pf := PDFFile{Path: destPath, OriginalName: safeName}
		if i < len(titles) {
			pf.Title = titles[i]
		}
		pdfFiles = append(pdfFiles, pf)
	}

	// Create output temp file
	outFile, err := os.CreateTemp("", "output-*.pdf")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Interner Fehler.")
		return
	}
	outPath := outFile.Name()
	outFile.Close()

	filename, err := processFolderMode(FolderModeOpts{
		PDFFiles:   pdfFiles,
		Title:      title,
		Subtitle:   subtitle,
		Font:       font,
		LineColor:  lineColor,
		BgColor:    bgColor,
		FontColor:  fontColor,
		Lang:       lang,
		DotLeaders: dotLeaders,
		ShowLines:  showLines,
		MergeOnly:  mergeOnly,
		OutputPath: outPath,
	})
	if err != nil {
		os.Remove(outPath)
		handleTocError(w, err)
		logFolderCall(tStart, len(files), title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, mergeOnly, lang, "error", err.Error())
		return
	}

	logFolderCall(tStart, len(files), title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, mergeOnly, lang, "success", "")
	servePDF(w, r, outPath, filename)
}

func handleGenerateJSON(w http.ResponseWriter, r *http.Request) {
	tStart := time.Now()

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "Ungültige Anfrage.")
		return
	}

	title := r.FormValue("title")
	subtitle := r.FormValue("subtitle")
	font := r.FormValue("font")
	dotLeaders := strings.ToLower(r.FormValue("dot_leaders")) != "false"
	showLines := strings.ToLower(r.FormValue("show_lines")) != "false"
	lineColor := r.FormValue("line_color")
	bgColor := r.FormValue("bg_color")
	fontColor := r.FormValue("font_color")
	lang := r.FormValue("lang")
	if lang == "" {
		lang = "de"
	}

	// Get PDF file
	pdfFile, pdfHeader, err := r.FormFile("pdf")
	if err != nil {
		writeError(w, http.StatusBadRequest, "PDF-Datei fehlt.")
		logJSONCall(tStart, title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, lang, "error", "PDF-Datei fehlt.")
		return
	}
	defer pdfFile.Close()

	if !strings.HasSuffix(strings.ToLower(pdfHeader.Filename), ".pdf") {
		msg := fmt.Sprintf("'%s' ist keine PDF-Datei.", pdfHeader.Filename)
		writeError(w, http.StatusBadRequest, msg)
		logJSONCall(tStart, title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, lang, "error", msg)
		return
	}

	// Get JSON file
	jsonFile, jsonHeader, err := r.FormFile("json_file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "JSON-Datei fehlt.")
		logJSONCall(tStart, title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, lang, "error", "JSON-Datei fehlt.")
		return
	}
	defer jsonFile.Close()

	if !strings.HasSuffix(strings.ToLower(jsonHeader.Filename), ".json") {
		msg := fmt.Sprintf("'%s' ist keine JSON-Datei.", jsonHeader.Filename)
		writeError(w, http.StatusBadRequest, msg)
		logJSONCall(tStart, title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, lang, "error", msg)
		return
	}

	// Save PDF to temp dir
	uploadDir, err := os.MkdirTemp("", "upload-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Interner Fehler.")
		return
	}
	defer os.RemoveAll(uploadDir)

	pdfPath := filepath.Join(uploadDir, filepath.Base(pdfHeader.Filename))
	if err := saveUploadFile(pdfFile, pdfHeader, pdfPath); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		logJSONCall(tStart, title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, lang, "error", err.Error())
		return
	}

	// Read JSON data
	jsonData, err := io.ReadAll(io.LimitReader(jsonFile, maxJSONBytes+1))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Fehler beim Lesen der JSON-Datei.")
		logJSONCall(tStart, title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, lang, "error", err.Error())
		return
	}
	if int64(len(jsonData)) > maxJSONBytes {
		msg := fmt.Sprintf("JSON-Datei zu groß (max %d MB).", maxJSONMB)
		writeError(w, http.StatusRequestEntityTooLarge, msg)
		logJSONCall(tStart, title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, lang, "error", msg)
		return
	}

	// Validate JSON is parseable and is a list
	var raw interface{}
	if err := json.Unmarshal(jsonData, &raw); err != nil {
		msg := fmt.Sprintf("Ungültiges JSON: %s", err.Error())
		writeError(w, http.StatusBadRequest, msg)
		logJSONCall(tStart, title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, lang, "error", msg)
		return
	}
	if _, ok := raw.([]interface{}); !ok {
		msg := "JSON muss eine Liste von Einträgen sein."
		writeError(w, http.StatusBadRequest, msg)
		logJSONCall(tStart, title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, lang, "error", msg)
		return
	}

	// Create output temp file
	outFile, err := os.CreateTemp("", "output-*.pdf")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Interner Fehler.")
		return
	}
	outPath := outFile.Name()
	outFile.Close()

	filename, err := processJSONMode(JSONModeOpts{
		PDFPath:    pdfPath,
		JSONData:   jsonData,
		Title:      title,
		Subtitle:   subtitle,
		Font:       font,
		LineColor:  lineColor,
		BgColor:    bgColor,
		FontColor:  fontColor,
		Lang:       lang,
		DotLeaders: dotLeaders,
		ShowLines:  showLines,
		OutputPath: outPath,
	})
	if err != nil {
		os.Remove(outPath)
		handleTocError(w, err)
		logJSONCall(tStart, title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, lang, "error", err.Error())
		return
	}

	logJSONCall(tStart, title, font, dotLeaders, showLines, lineColor, bgColor, fontColor, lang, "success", "")
	servePDF(w, r, outPath, filename)
}

func servePDF(w http.ResponseWriter, r *http.Request, filePath, filename string) {
	defer os.Remove(filePath)

	f, err := os.Open(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Fehler beim Lesen der Ausgabedatei.")
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Fehler beim Lesen der Ausgabedatei.")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	io.Copy(w, f)
}

// ──────────────────────────────────────────────
// Logging helpers
// ──────────────────────────────────────────────

func logFolderCall(tStart time.Time, fileCount int, title, font string,
	dotLeaders, showLines bool, lineColor, bgColor, fontColor string, mergeOnly bool,
	lang, status, errMsg string) {

	durationMs := int(time.Since(tStart).Milliseconds())
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	logCall(LogCallParams{
		Endpoint:   "/api/generate/folder",
		Mode:       "folder",
		FileCount:  fileCount,
		Title:      ptrStr(title),
		Font:       ptrStr(font),
		DotLeaders: dotLeaders,
		ShowLines:  showLines,
		LineColor:  ptrStr(lineColor),
		BgColor:    ptrStr(bgColor),
		FontColor:  ptrStr(fontColor),
		MergeOnly:  mergeOnly,
		Lang:       ptrStr(lang),
		Status:     status,
		DurationMs: durationMs,
		Error:      errPtr,
	})
}

func logJSONCall(tStart time.Time, title, font string,
	dotLeaders, showLines bool, lineColor, bgColor, fontColor, lang, status, errMsg string) {

	durationMs := int(time.Since(tStart).Milliseconds())
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	logCall(LogCallParams{
		Endpoint:   "/api/generate/json",
		Mode:       "json",
		FileCount:  1,
		Title:      ptrStr(title),
		Font:       ptrStr(font),
		DotLeaders: dotLeaders,
		ShowLines:  showLines,
		LineColor:  ptrStr(lineColor),
		BgColor:    ptrStr(bgColor),
		FontColor:  ptrStr(fontColor),
		Lang:       ptrStr(lang),
		Status:     status,
		DurationMs: durationMs,
		Error:      errPtr,
	})
}

// ──────────────────────────────────────────────
// Exec helper with timeout
// ──────────────────────────────────────────────

func runCommandWithTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	done := make(chan struct{})
	cmd := exec.Command(name, args...)
	var out []byte
	var cmdErr error
	go func() {
		out, cmdErr = cmd.Output()
		close(done)
	}()
	select {
	case <-done:
		return out, cmdErr
	case <-time.After(timeout):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return nil, fmt.Errorf("timeout after %v", timeout)
	}
}

// ──────────────────────────────────────────────
// Main
// ──────────────────────────────────────────────

// runHealthCheck probes the local /health endpoint and reports the result via
// the exit code. It exists so the container image does not need wget or curl
// just to answer Docker's HEALTHCHECK.
func runHealthCheck() int {
	client := &http.Client{Timeout: healthCheckTimeout}
	resp, err := client.Get("http://127.0.0.1:8000/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(runHealthCheck())
	}

	slog.Info("starting toc-service", "port", 8000)

	if err := initDB(); err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /api/fonts", handleFonts)
	mux.HandleFunc("GET /api/colors", handleColors)
	mux.HandleFunc("GET /api/bg-colors", handleBgColors)
	mux.HandleFunc("GET /api/font-colors", handleFontColors)
	mux.HandleFunc("GET /usage", handleUsagePage)
	mux.HandleFunc("GET /howto", handleHowToPage)
	mux.HandleFunc("GET /local", handleLocalPage)
	mux.HandleFunc("GET /api/usage", handleUsageAPI)
	mux.HandleFunc("POST /api/generate/folder", handleGenerateFolder)
	mux.HandleFunc("POST /api/generate/json", handleGenerateJSON)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("GET /fonts/", http.StripPrefix("/fonts/", http.FileServer(http.Dir("fonts"))))
	mux.HandleFunc("GET /", handleRoot)

	handler := requestIDMiddleware(mux)

	server := &http.Server{
		Addr:              ":8000",
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
