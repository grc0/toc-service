package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"os/exec"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTestServer(t *testing.T) *http.ServeMux {
	t.Helper()

	// Initialize DB in temp dir
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(origDir) })

	// Create required directories
	os.MkdirAll("templates", 0o755)
	os.MkdirAll("static", 0o755)

	// Copy template
	templateSrc := filepath.Join(origDir, "templates", "toc_template.typ")
	templateData, err := os.ReadFile(templateSrc)
	if err != nil {
		t.Skipf("template not found at %s: %v", templateSrc, err)
	}
	os.WriteFile(filepath.Join("templates", "toc_template.typ"), templateData, 0o644)

	// Create minimal static files
	os.WriteFile(filepath.Join("static", "index.html"), []byte("<html>test</html>"), 0o644)
	os.WriteFile(filepath.Join("static", "usage.html"), []byte("<html>usage</html>"), 0o644)
	os.WriteFile(filepath.Join("static", "howto.html"), []byte("<html>howto</html>"), 0o644)

	if err := initDB(); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleRoot)
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /api/fonts", handleFonts)
	mux.HandleFunc("GET /api/colors", handleColors)
	mux.HandleFunc("GET /api/bg-colors", handleBgColors)
	mux.HandleFunc("GET /api/font-colors", handleFontColors)
	mux.HandleFunc("GET /usage", handleUsagePage)
	mux.HandleFunc("GET /howto", handleHowToPage)
	mux.HandleFunc("GET /api/usage", handleUsageAPI)
	mux.HandleFunc("POST /api/generate/folder", handleGenerateFolder)
	mux.HandleFunc("POST /api/generate/json", handleGenerateJSON)

	return mux
}

// ──────────────────────────────────────────────
// GET endpoints
// ──────────────────────────────────────────────

func TestServeFrontend(t *testing.T) {
	mux := setupTestServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct == "" {
		t.Error("expected Content-Type header")
	}
}

func TestHealthCheck(t *testing.T) {
	mux := setupTestServer(t)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var data map[string]string
	json.NewDecoder(w.Body).Decode(&data)
	if data["status"] != "ok" {
		t.Errorf("status = %q, want ok", data["status"])
	}
	if _, ok := data["typst_version"]; !ok {
		t.Error("expected typst_version in response")
	}
}

func TestListFonts(t *testing.T) {
	mux := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/fonts", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var data map[string]interface{}
	json.NewDecoder(w.Body).Decode(&data)
	if data["default"] != "IBM Plex Sans" {
		t.Errorf("default font = %v, want IBM Plex Sans", data["default"])
	}
	fonts, ok := data["fonts"].([]interface{})
	if !ok || len(fonts) == 0 {
		t.Error("expected non-empty fonts list")
	}
}

func TestListColors(t *testing.T) {
	mux := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/colors", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var data map[string]interface{}
	json.NewDecoder(w.Body).Decode(&data)
	if data["default"] != "#1A2980" {
		t.Errorf("default color = %v, want #1A2980", data["default"])
	}
}

func TestHandleBgColors(t *testing.T) {
	mux := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/bg-colors", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var data map[string]interface{}
	json.NewDecoder(w.Body).Decode(&data)
	if data["default"] != "#FFFFFF" {
		t.Errorf("default bg color = %v, want #FFFFFF", data["default"])
	}
	colors, ok := data["colors"].(map[string]interface{})
	if !ok || len(colors) == 0 {
		t.Error("expected non-empty colors map")
	}
}

func TestHandleFontColors(t *testing.T) {
	mux := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/font-colors", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var data map[string]interface{}
	json.NewDecoder(w.Body).Decode(&data)
	if data["default"] != "#000000" {
		t.Errorf("default font color = %v, want #000000", data["default"])
	}
	colors, ok := data["colors"].(map[string]interface{})
	if !ok || len(colors) == 0 {
		t.Error("expected non-empty colors map")
	}
}

func TestUsageAPI(t *testing.T) {
	mux := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/usage?limit=10", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var data map[string]interface{}
	json.NewDecoder(w.Body).Decode(&data)
	if _, ok := data["stats"]; !ok {
		t.Error("expected stats in response")
	}
	if _, ok := data["recent"]; !ok {
		t.Error("expected recent in response")
	}
}

// ──────────────────────────────────────────────
// Request ID middleware
// ──────────────────────────────────────────────

func TestRequestIDGenerated(t *testing.T) {
	mux := setupTestServer(t)
	handler := requestIDMiddleware(mux)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header")
	}
}

func TestRequestIDPreserved(t *testing.T) {
	mux := setupTestServer(t)
	handler := requestIDMiddleware(mux)

	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("X-Request-ID", "test-123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != "test-123" {
		t.Errorf("X-Request-ID = %q, want test-123", got)
	}
}

// ──────────────────────────────────────────────
// writeError
// ──────────────────────────────────────────────

func TestWriteErrorClient(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "bad input")

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var data map[string]string
	json.NewDecoder(w.Body).Decode(&data)
	if data["detail"] != "bad input" {
		t.Errorf("detail = %q, want 'bad input'", data["detail"])
	}
	if _, hasError := data["error"]; hasError {
		t.Error("client errors should use 'detail' not 'error'")
	}
}

func TestWriteErrorServer(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusInternalServerError, "server broke")

	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
	var data map[string]string
	json.NewDecoder(w.Body).Decode(&data)
	if data["error"] != "server broke" {
		t.Errorf("error = %q, want 'server broke'", data["error"])
	}
}

// ──────────────────────────────────────────────
// handleTocError
// ──────────────────────────────────────────────

func TestHandleTocErrorNoPdfs(t *testing.T) {
	w := httptest.NewRecorder()
	handleTocError(w, &NoPdfsFoundError{Message: "no pdfs"})
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleTocErrorInvalidJSON(t *testing.T) {
	w := httptest.NewRecorder()
	handleTocError(w, &InvalidJsonError{Message: "bad json"})
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleTocErrorTypst(t *testing.T) {
	w := httptest.NewRecorder()
	handleTocError(w, &TypstCompilationError{Message: "typst fail"})
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestHandleTocErrorGeneric(t *testing.T) {
	w := httptest.NewRecorder()
	handleTocError(w, &TocError{Message: "generic toc"})
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestHandleTocErrorDefault(t *testing.T) {
	w := httptest.NewRecorder()
	handleTocError(w, fmt.Errorf("unexpected"))
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ──────────────────────────────────────────────
// ptrStr
// ──────────────────────────────────────────────

func TestPtrStrEmpty(t *testing.T) {
	if ptrStr("") != nil {
		t.Error("ptrStr('') should be nil")
	}
}

func TestPtrStrNonEmpty(t *testing.T) {
	p := ptrStr("hello")
	if p == nil || *p != "hello" {
		t.Errorf("ptrStr('hello') = %v", p)
	}
}

// ──────────────────────────────────────────────
// handleUsagePage
// ──────────────────────────────────────────────

func TestHandleUsagePage(t *testing.T) {
	mux := setupTestServer(t)
	req := httptest.NewRequest("GET", "/usage", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// ──────────────────────────────────────────────
// handleHowToPage
// ──────────────────────────────────────────────

func TestHandleHowToPage(t *testing.T) {
	mux := setupTestServer(t)
	req := httptest.NewRequest("GET", "/howto", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// ──────────────────────────────────────────────
// handleRoot 404 for non-root paths
// ──────────────────────────────────────────────

func TestHandleRootNotFound(t *testing.T) {
	mux := setupTestServer(t)
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// ──────────────────────────────────────────────
// saveUploadFile
// ──────────────────────────────────────────────

// fakeMultipartFile wraps bytes.Reader to satisfy multipart.File
type fakeMultipartFile struct {
	*bytes.Reader
}

func (f *fakeMultipartFile) Close() error { return nil }

func TestSaveUploadFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "uploaded.pdf")

	content := []byte("fake pdf content")
	file := &fakeMultipartFile{bytes.NewReader(content)}

	err := saveUploadFile(file, nil, dest)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "fake pdf content" {
		t.Errorf("content = %q", string(data))
	}
}

// ──────────────────────────────────────────────
// servePDF
// ──────────────────────────────────────────────

func TestServePDF(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	os.WriteFile(pdfPath, []byte("fake-pdf-data"), 0o644)

	req := httptest.NewRequest("GET", "/download", nil)
	w := httptest.NewRecorder()

	servePDF(w, req, pdfPath, "output.pdf")

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "output.pdf") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if w.Body.String() != "fake-pdf-data" {
		t.Errorf("body = %q", w.Body.String())
	}

	// File should be removed after serving
	if _, err := os.Stat(pdfPath); !os.IsNotExist(err) {
		t.Error("expected PDF file to be removed after serving")
	}
}

func TestServePDFMissingFile(t *testing.T) {
	req := httptest.NewRequest("GET", "/download", nil)
	w := httptest.NewRecorder()

	servePDF(w, req, "/nonexistent/file.pdf", "output.pdf")

	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ──────────────────────────────────────────────
// handleGenerateFolder — error cases
// ──────────────────────────────────────────────

func TestHandleGenerateFolderNoFiles(t *testing.T) {
	mux := setupTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("title", "Test")
	writer.Close()

	req := httptest.NewRequest("POST", "/api/generate/folder", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleGenerateFolderNonPDF(t *testing.T) {
	mux := setupTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("title", "Test")
	part, _ := writer.CreateFormFile("files", "doc.txt")
	part.Write([]byte("not a pdf"))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/generate/folder", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var data map[string]string
	json.NewDecoder(w.Body).Decode(&data)
	if !strings.Contains(data["detail"], "keine PDF") {
		t.Errorf("detail = %q, expected PDF error", data["detail"])
	}
}

// ──────────────────────────────────────────────
// handleGenerateJSON — error cases
// ──────────────────────────────────────────────

func TestHandleGenerateJSONMissingPDF(t *testing.T) {
	mux := setupTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("title", "Test")
	writer.Close()

	req := httptest.NewRequest("POST", "/api/generate/json", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleGenerateJSONNonPDF(t *testing.T) {
	mux := setupTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("title", "Test")
	part, _ := writer.CreateFormFile("pdf", "doc.txt")
	part.Write([]byte("not a pdf"))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/generate/json", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleGenerateJSONMissingJSONFile(t *testing.T) {
	mux := setupTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("title", "Test")
	part, _ := writer.CreateFormFile("pdf", "doc.pdf")
	part.Write([]byte("fake pdf"))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/generate/json", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleGenerateJSONNonJSONFile(t *testing.T) {
	mux := setupTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("title", "Test")
	part, _ := writer.CreateFormFile("pdf", "doc.pdf")
	part.Write([]byte("fake pdf"))
	part2, _ := writer.CreateFormFile("json_file", "data.txt")
	part2.Write([]byte("not json"))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/generate/json", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleGenerateJSONInvalidJSON(t *testing.T) {
	mux := setupTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("title", "Test")
	part, _ := writer.CreateFormFile("pdf", "doc.pdf")
	part.Write([]byte("fake pdf"))
	part2, _ := writer.CreateFormFile("json_file", "data.json")
	part2.Write([]byte("{invalid json"))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/generate/json", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleGenerateJSONNotArray(t *testing.T) {
	mux := setupTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("title", "Test")
	part, _ := writer.CreateFormFile("pdf", "doc.pdf")
	part.Write([]byte("fake pdf"))
	part2, _ := writer.CreateFormFile("json_file", "data.json")
	part2.Write([]byte(`{"not": "array"}`))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/generate/json", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// ──────────────────────────────────────────────
// Usage API with limit
// ──────────────────────────────────────────────

func TestUsageAPIDefaultLimit(t *testing.T) {
	mux := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/usage", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestUsageAPIInvalidLimit(t *testing.T) {
	mux := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/usage?limit=abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200 (invalid limit should use default)", w.Code)
	}
}

// ──────────────────────────────────────────────
// boolToInt
// ──────────────────────────────────────────────

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Error("boolToInt(true) should be 1")
	}
	if boolToInt(false) != 0 {
		t.Error("boolToInt(false) should be 0")
	}
}

// ──────────────────────────────────────────────
// nullStr
// ──────────────────────────────────────────────

func TestNullStrValid(t *testing.T) {
	ns := sql.NullString{String: "hello", Valid: true}
	if got := nullStr(ns); got != "hello" {
		t.Errorf("nullStr(valid) = %v, want 'hello'", got)
	}
}

func TestNullStrInvalid(t *testing.T) {
	ns := sql.NullString{Valid: false}
	if got := nullStr(ns); got != nil {
		t.Errorf("nullStr(invalid) = %v, want nil", got)
	}
}

// ──────────────────────────────────────────────
// logCall integration
// ──────────────────────────────────────────────

func TestLogCallAndRetrieve(t *testing.T) {
	setupTestServer(t) // initializes DB in temp dir

	title := "Test Title"
	errMsg := "some error"
	logCall(LogCallParams{
		Endpoint:   "/api/generate/folder",
		Mode:       "folder",
		FileCount:  3,
		Title:      &title,
		DotLeaders: true,
		ShowLines:  true,
		Status:     "error",
		DurationMs: 123,
		Error:      &errMsg,
	})

	recent, err := getRecentCalls(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) < 1 {
		t.Fatal("expected at least 1 recent call")
	}
	last := recent[0]
	if last["mode"] != "folder" {
		t.Errorf("mode = %v, want folder", last["mode"])
	}
	if last["file_count"] != 3 {
		t.Errorf("file_count = %v, want 3", last["file_count"])
	}
	if last["title"] != "Test Title" {
		t.Errorf("title = %v, want 'Test Title'", last["title"])
	}
	if last["status"] != "error" {
		t.Errorf("status = %v, want error", last["status"])
	}

	stats, err := getStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats["total_calls"].(int) < 1 {
		t.Error("expected total_calls >= 1")
	}
	if stats["errors"].(int) < 1 {
		t.Error("expected errors >= 1")
	}
}

func TestLogFolderCall(t *testing.T) {
	setupTestServer(t)

	logFolderCall(
		time.Now(),
		5, "Title", "Roboto", true, false, "#FF0000", "", "", false, "en", "success", "",
	)

	recent, err := getRecentCalls(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) < 1 {
		t.Fatal("expected at least 1 call")
	}
	if recent[0]["endpoint"] != "/api/generate/folder" {
		t.Errorf("endpoint = %v", recent[0]["endpoint"])
	}
}

func TestLogJSONCall(t *testing.T) {
	setupTestServer(t)

	logJSONCall(
		time.Now(),
		"Title", "Inter", true, true, "#000000", "", "", "de", "success", "",
	)

	recent, err := getRecentCalls(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) < 1 {
		t.Fatal("expected at least 1 call")
	}
	if recent[0]["endpoint"] != "/api/generate/json" {
		t.Errorf("endpoint = %v", recent[0]["endpoint"])
	}
}

// ──────────────────────────────────────────────
// handleGenerateFolder — valid upload (requires typst)
// ──────────────────────────────────────────────

func TestHandleGenerateFolderValid(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not installed, skipping")
	}
	mux := setupTestServer(t)

	// Create a valid test PDF
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDFForHandler(t, pdfPath, 3)
	pdfData, _ := os.ReadFile(pdfPath)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("title", "Handler Test")
	writer.WriteField("dot_leaders", "true")
	writer.WriteField("show_lines", "true")
	writer.WriteField("line_color", "#FF0000")
	writer.WriteField("font", "Roboto")
	writer.WriteField("lang", "en")
	part, _ := writer.CreateFormFile("files", "test.pdf")
	part.Write(pdfData)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/generate/folder", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
}

func TestHandleGenerateFolderMergeOnly(t *testing.T) {
	mux := setupTestServer(t)

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDFForHandler(t, pdfPath, 2)
	pdfData, _ := os.ReadFile(pdfPath)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("title", "Merge Test")
	writer.WriteField("merge_only", "true")
	part, _ := writer.CreateFormFile("files", "a.pdf")
	part.Write(pdfData)
	part2, _ := writer.CreateFormFile("files", "b.pdf")
	part2.Write(pdfData)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/generate/folder", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
}

// ──────────────────────────────────────────────
// handleGenerateJSON — valid upload (requires typst)
// ──────────────────────────────────────────────

func TestHandleGenerateJSONValid(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not installed, skipping")
	}
	mux := setupTestServer(t)

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDFForHandler(t, pdfPath, 10)
	pdfData, _ := os.ReadFile(pdfPath)

	jsonContent := `[{"title": "Chapter 1", "page": 1, "pages": 5}, {"title": "Chapter 2", "page": 6, "pages": 5}]`

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("title", "JSON Handler Test")
	writer.WriteField("dot_leaders", "false")
	writer.WriteField("show_lines", "false")
	writer.WriteField("font", "Inter")
	writer.WriteField("lang", "de")
	pdfPart, _ := writer.CreateFormFile("pdf", "source.pdf")
	pdfPart.Write(pdfData)
	jsonPart, _ := writer.CreateFormFile("json_file", "toc.json")
	jsonPart.Write([]byte(jsonContent))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/generate/json", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
}

// createTestPDFForHandler reuses the same logic as createTestPDF from toc_engine_test.go
func createTestPDFForHandler(t *testing.T, path string, numPages int) {
	t.Helper()

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	objNum := 1
	offsets := make([]int, 0)

	offsets = append(offsets, buf.Len())
	fmt.Fprintf(&buf, "%d 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n", objNum)
	objNum++

	offsets = append(offsets, buf.Len())
	kids := make([]string, numPages)
	for i := 0; i < numPages; i++ {
		kids[i] = fmt.Sprintf("%d 0 R", 3+i)
	}
	fmt.Fprintf(&buf, "%d 0 obj\n<</Type /Pages /Kids [%s] /Count %d>>\nendobj\n",
		objNum, strings.Join(kids, " "), numPages)
	objNum++

	for i := 0; i < numPages; i++ {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n<</Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources <<>>>>\nendobj\n", objNum)
		objNum++
	}

	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", objNum)
	fmt.Fprintf(&buf, "0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<</Size %d /Root 1 0 R>>\n", objNum)
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLogFolderCallWithError(t *testing.T) {
	setupTestServer(t)

	logFolderCall(
		time.Now(),
		2, "Doc", "Inter", false, true, "#000000", "#F3F4F6", "", true, "de", "error", "something failed",
	)

	recent, err := getRecentCalls(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) < 1 {
		t.Fatal("expected at least 1 call")
	}
	if recent[0]["status"] != "error" {
		t.Errorf("status = %v, want error", recent[0]["status"])
	}
}
