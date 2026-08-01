package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
)

// ──────────────────────────────────────────────
// Test PDF helper
// ──────────────────────────────────────────────

func createTestPDF(t *testing.T, path string, numPages int) {
	t.Helper()

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	objNum := 1
	offsets := make([]int, 0)

	// Object 1: Catalog
	offsets = append(offsets, buf.Len())
	fmt.Fprintf(&buf, "%d 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n", objNum)
	objNum++

	// Object 2: Pages
	offsets = append(offsets, buf.Len())
	kids := make([]string, numPages)
	for i := 0; i < numPages; i++ {
		kids[i] = fmt.Sprintf("%d 0 R", 3+i)
	}
	fmt.Fprintf(&buf, "%d 0 obj\n<</Type /Pages /Kids [%s] /Count %d>>\nendobj\n",
		objNum, strings.Join(kids, " "), numPages)
	objNum++

	// Individual pages
	for i := 0; i < numPages; i++ {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n<</Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources <<>>>>\nendobj\n", objNum)
		objNum++
	}

	// Cross-reference table
	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", objNum)
	fmt.Fprintf(&buf, "0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}

	// Trailer
	fmt.Fprintf(&buf, "trailer\n<</Size %d /Root 1 0 R>>\n", objNum)
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ──────────────────────────────────────────────
// validateColor
// ──────────────────────────────────────────────

func TestValidateColorNormalizes(t *testing.T) {
	tests := []struct{ input, want string }{
		{"1a2980", "#1A2980"},
		{"#1a2980", "#1A2980"},
		{"#ABCDEF", "#ABCDEF"},
	}
	for _, tt := range tests {
		if got := validateColor(tt.input); got != tt.want {
			t.Errorf("validateColor(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidateColorInvalid(t *testing.T) {
	tests := []string{"zzz", "#123", "", "#1234567"}
	for _, input := range tests {
		if got := validateColor(input); got != DefaultLineColor {
			t.Errorf("validateColor(%q) = %q, want default %q", input, got, DefaultLineColor)
		}
	}
}

func TestValidateColorWhitespace(t *testing.T) {
	if got := validateColor("  #ff0000  "); got != "#FF0000" {
		t.Errorf("validateColor with whitespace = %q, want #FF0000", got)
	}
}

// ──────────────────────────────────────────────
// folderToTitle
// ──────────────────────────────────────────────

func TestFolderToTitle(t *testing.T) {
	tests := []struct{ input, want string }{
		{"arbeitszeugnisse", "Arbeitszeugnisse"},
		{"aus_und_weiterbildungen", "Aus- und Weiterbildungen"},
		{"brief_von_firma", "Brief von Firma"},
		{"meine-dokumente", "Meine Dokumente"},
		{"too__many___spaces", "Too Many Spaces"},
		{"", "Inhaltsverzeichnis"},
	}
	for _, tt := range tests {
		if got := folderToTitle(tt.input); got != tt.want {
			t.Errorf("folderToTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ──────────────────────────────────────────────
// cleanTitle
// ──────────────────────────────────────────────

func TestCleanTitle(t *testing.T) {
	tests := []struct{ input, want string }{
		{"my_document.pdf", "my document"},
		{"01_intro.pdf", "intro"},
		{"001-chapter.pdf", "– chapter"},
		{"42.summary.pdf", "summary"},
		{"part-one.pdf", "part – one"},
		{"readme", "readme"},
	}
	for _, tt := range tests {
		if got := cleanTitle(tt.input); got != tt.want {
			t.Errorf("cleanTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ──────────────────────────────────────────────
// titleToFilename
// ──────────────────────────────────────────────

func TestTitleToFilename(t *testing.T) {
	tests := []struct{ input, want string }{
		{"My Document", "My Document.pdf"},
		{`Test: "quotes" <angle>`, "Test quotes angle.pdf"},
		{"", "ausgabe.pdf"},
		{`<>:"/\|?*`, "ausgabe.pdf"},
	}
	for _, tt := range tests {
		if got := titleToFilename(tt.input); got != tt.want {
			t.Errorf("titleToFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ──────────────────────────────────────────────
// computeTitleBlock
// ──────────────────────────────────────────────

func TestComputeTitleBlock(t *testing.T) {
	if got := computeTitleBlock(true, true, true); got != 126.3 {
		t.Errorf("all enabled = %v, want 126.3", got)
	}
	if got := computeTitleBlock(false, false, false); got != 37.3 {
		t.Errorf("nothing = %v, want 37.3", got)
	}
	// All 8 combinations should return a value
	for _, lines := range []bool{true, false} {
		for _, title := range []bool{true, false} {
			for _, subtitle := range []bool{true, false} {
				result := computeTitleBlock(title, subtitle, lines)
				if result == 0 {
					t.Errorf("computeTitleBlock(%v, %v, %v) = 0", title, subtitle, lines)
				}
			}
		}
	}
}

// ──────────────────────────────────────────────
// escapeTypst
// ──────────────────────────────────────────────

func TestEscapeTypst(t *testing.T) {
	tests := []struct{ input, want string }{
		{"Hello World", "Hello World"},
		{"#read()", "\\#read()"},
		{"[injected]", "\\[injected\\]"},
		{`say "hello"`, `say \"hello\"`},
		{`path\to`, `path\\to`},
	}
	for _, tt := range tests {
		if got := escapeTypst(tt.input); got != tt.want {
			t.Errorf("escapeTypst(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEscapeTypstInjection(t *testing.T) {
	payload := "]\n#text(read(\"/etc/passwd\"))\n#text["
	escaped := escapeTypst(payload)
	want := "\\]\n\\#text(read(\\\"/etc/passwd\\\"))\n\\#text\\["
	if escaped != want {
		t.Errorf("escapeTypst injection:\ngot:  %q\nwant: %q", escaped, want)
	}
}

// ──────────────────────────────────────────────
// loadJSONEntries
// ──────────────────────────────────────────────

func TestLoadJSONEntriesValid(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDF(t, pdfPath, 5)

	data := []interface{}{
		map[string]interface{}{"title": "A", "page": float64(1)},
		map[string]interface{}{"title": "B", "page": float64(3)},
	}
	entries, err := loadJSONEntries(data, pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Page != 1 || entries[0].Pages != 2 {
		t.Errorf("entry 0: page=%d pages=%d, want page=1 pages=2", entries[0].Page, entries[0].Pages)
	}
	if entries[1].Pages != 3 {
		t.Errorf("entry 1: pages=%d, want 3", entries[1].Pages)
	}
}

func TestLoadJSONEntriesOutOfBounds(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDF(t, pdfPath, 2)

	data := []interface{}{
		map[string]interface{}{"title": "A", "page": float64(3)},
	}
	_, err := loadJSONEntries(data, pdfPath)
	if err == nil {
		t.Fatal("expected error for out of bounds page")
	}
	var invalidJSON *InvalidJsonError
	if !isInvalidJSON(err) {
		t.Errorf("expected InvalidJsonError, got %T: %v", err, invalidJSON)
	}
}

func TestLoadJSONEntriesPageZero(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDF(t, pdfPath, 5)

	data := []interface{}{
		map[string]interface{}{"title": "A", "page": float64(0)},
	}
	_, err := loadJSONEntries(data, pdfPath)
	if err == nil {
		t.Fatal("expected error for page 0")
	}
}

func TestLoadJSONEntriesInvalidPageType(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDF(t, pdfPath, 2)

	data := []interface{}{
		map[string]interface{}{"title": "A", "page": "x"},
	}
	_, err := loadJSONEntries(data, pdfPath)
	if err == nil {
		t.Fatal("expected error for invalid page type")
	}
}

func TestLoadJSONEntriesInvalidPagesType(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDF(t, pdfPath, 5)

	data := []interface{}{
		map[string]interface{}{"title": "A", "page": float64(1), "pages": "many"},
	}
	_, err := loadJSONEntries(data, pdfPath)
	if err == nil {
		t.Fatal("expected error for invalid pages type")
	}
}

func TestLoadJSONEntriesEmptyList(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDF(t, pdfPath, 2)

	_, err := loadJSONEntries([]interface{}{}, pdfPath)
	if err == nil {
		t.Fatal("expected error for empty list")
	}
}

func TestLoadJSONEntriesMissingFields(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDF(t, pdfPath, 2)

	// Missing title
	_, err := loadJSONEntries([]interface{}{
		map[string]interface{}{"page": float64(1)},
	}, pdfPath)
	if err == nil {
		t.Fatal("expected error for missing title")
	}

	// Missing page
	_, err = loadJSONEntries([]interface{}{
		map[string]interface{}{"title": "A"},
	}, pdfPath)
	if err == nil {
		t.Fatal("expected error for missing page")
	}
}

func TestLoadJSONEntriesExplicitPages(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDF(t, pdfPath, 10)

	data := []interface{}{
		map[string]interface{}{"title": "A", "page": float64(1), "pages": float64(4)},
		map[string]interface{}{"title": "B", "page": float64(5), "pages": float64(6)},
	}
	entries, err := loadJSONEntries(data, pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Pages != 4 {
		t.Errorf("entry 0 pages = %d, want 4", entries[0].Pages)
	}
	if entries[1].Pages != 6 {
		t.Errorf("entry 1 pages = %d, want 6", entries[1].Pages)
	}
}

func TestLoadJSONEntriesSingleEntry(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDF(t, pdfPath, 5)

	data := []interface{}{
		map[string]interface{}{"title": "Only", "page": float64(1)},
	}
	entries, err := loadJSONEntries(data, pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Pages != 5 {
		t.Errorf("pages = %d, want 5", entries[0].Pages)
	}
}

// ──────────────────────────────────────────────
// collectPDFs
// ──────────────────────────────────────────────

func TestCollectPDFs(t *testing.T) {
	dir := t.TempDir()
	createTestPDF(t, filepath.Join(dir, "alpha.pdf"), 2)
	createTestPDF(t, filepath.Join(dir, "beta.pdf"), 3)

	entries, err := collectPDFs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Title == "" || e.Pages == 0 {
			t.Errorf("invalid entry: %+v", e)
		}
	}
}

func TestCollectPDFsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := collectPDFs(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

// ──────────────────────────────────────────────
// mergePDFs
// ──────────────────────────────────────────────

func TestMergePDFs(t *testing.T) {
	dir := t.TempDir()
	tocPath := filepath.Join(dir, "toc.pdf")
	aPath := filepath.Join(dir, "a.pdf")
	bPath := filepath.Join(dir, "b.pdf")
	outPath := filepath.Join(dir, "merged.pdf")

	createTestPDF(t, tocPath, 1)
	createTestPDF(t, aPath, 2)
	createTestPDF(t, bPath, 3)

	if err := mergePDFFiles([]string{tocPath, aPath, bPath}, outPath); err != nil {
		t.Fatal(err)
	}

	pages, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 6 {
		t.Errorf("merged page count = %d, want 6", pages)
	}
}

func TestMergeWithExistingPDF(t *testing.T) {
	dir := t.TempDir()
	tocPath := filepath.Join(dir, "toc.pdf")
	existingPath := filepath.Join(dir, "existing.pdf")
	outPath := filepath.Join(dir, "merged.pdf")

	createTestPDF(t, tocPath, 1)
	createTestPDF(t, existingPath, 5)

	if err := mergePDFFiles([]string{tocPath, existingPath}, outPath); err != nil {
		t.Fatal(err)
	}

	pages, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 6 {
		t.Errorf("merged page count = %d, want 6", pages)
	}
}

// ──────────────────────────────────────────────
// addBookmarks
// ──────────────────────────────────────────────

func TestAddBookmarks(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "input.pdf")
	outPath := filepath.Join(dir, "output.pdf")
	createTestPDF(t, inPath, 5)

	entries := []TocEntry{
		{Title: "Chapter 1", FinalPage: 2},
		{Title: "Chapter 2", FinalPage: 4},
	}
	if err := addBookmarks(inPath, outPath, entries, "Contents"); err != nil {
		t.Fatal(err)
	}

	// Verify bookmarks exist by reading them back
	f, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	bms, err := api.Bookmarks(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(bms) == 0 {
		t.Error("expected bookmarks, got none")
	}
}

// ──────────────────────────────────────────────
// extractOutline
// ──────────────────────────────────────────────

func TestExtractOutlineEmpty(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "no_bookmarks.pdf")
	createTestPDF(t, pdfPath, 2)

	outline, err := extractOutline(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(outline) != 0 {
		t.Errorf("expected empty outline, got %d items", len(outline))
	}
}

func TestExtractOutlineWithBookmarks(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "base.pdf")
	withBmPath := filepath.Join(dir, "with_bm.pdf")
	createTestPDF(t, inPath, 5)

	// Add bookmarks using pdfcpu
	bms := []pdfcpu.Bookmark{
		{Title: "Intro", PageFrom: 1},
		{Title: "Chapter 1", PageFrom: 3},
		{Title: "Chapter 2", PageFrom: 5},
	}
	if err := api.AddBookmarksFile(inPath, withBmPath, bms, true, nil); err != nil {
		t.Fatal(err)
	}

	outline, err := extractOutline(withBmPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(outline) != 3 {
		t.Fatalf("expected 3 outline items, got %d", len(outline))
	}
	if outline[0].Title != "Intro" || outline[0].Page != 0 {
		t.Errorf("outline[0] = %+v, want Intro page 0", outline[0])
	}
	if outline[1].Title != "Chapter 1" || outline[1].Page != 2 {
		t.Errorf("outline[1] = %+v, want Chapter 1 page 2", outline[1])
	}
}

// ──────────────────────────────────────────────
// Folder mode merge-only
// ──────────────────────────────────────────────

func TestProcessFolderModeMergeOnly(t *testing.T) {
	dir := t.TempDir()
	pdf1 := filepath.Join(dir, "doc1.pdf")
	pdf2 := filepath.Join(dir, "doc2.pdf")
	outPath := filepath.Join(dir, "output.pdf")
	createTestPDF(t, pdf1, 2)
	createTestPDF(t, pdf2, 3)

	filename, err := processFolderMode(FolderModeOpts{
		PDFFiles: []PDFFile{
			{Path: pdf1, OriginalName: "doc1.pdf"},
			{Path: pdf2, OriginalName: "doc2.pdf"},
		},
		Title:      "Test",
		MergeOnly:  true,
		OutputPath: outPath,
	})
	if err != nil {
		t.Fatal(err)
	}

	if filename != "Test.pdf" {
		t.Errorf("filename = %q, want Test.pdf", filename)
	}

	pages, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 5 {
		t.Errorf("page count = %d, want 5", pages)
	}
}

// ──────────────────────────────────────────────
// helper
// ──────────────────────────────────────────────

// ──────────────────────────────────────────────
// truncateEntry
// ──────────────────────────────────────────────

func TestTruncateEntryShort(t *testing.T) {
	input := "Short title"
	if got := truncateEntry(input); got != input {
		t.Errorf("truncateEntry(%q) = %q, want unchanged", input, got)
	}
}

func TestTruncateEntryExact(t *testing.T) {
	// Exactly 80 characters — should not truncate
	input := strings.Repeat("A", 80)
	if got := truncateEntry(input); got != input {
		t.Errorf("truncateEntry(80 chars) was truncated")
	}
}

func TestTruncateEntryLong(t *testing.T) {
	input := strings.Repeat("X", 100)
	got := truncateEntry(input)
	expected := strings.Repeat("X", 80) + "..."
	if got != expected {
		t.Errorf("truncateEntry(100 chars) = %q, want 80+...", got)
	}
}

func TestTruncateEntryUnicode(t *testing.T) {
	// 81 runes of multi-byte characters
	input := strings.Repeat("ä", 81)
	got := truncateEntry(input)
	if len([]rune(got)) != 83 { // 80 runes + "..."
		t.Errorf("truncateEntry unicode: got %d runes, want 83", len([]rune(got)))
	}
}

// ──────────────────────────────────────────────
// coalesce
// ──────────────────────────────────────────────

func TestCoalesceFirst(t *testing.T) {
	if got := coalesce("a", "b"); got != "a" {
		t.Errorf("coalesce(a,b) = %q, want a", got)
	}
}

func TestCoalesceSkipsEmpty(t *testing.T) {
	if got := coalesce("", "b", "c"); got != "b" {
		t.Errorf("coalesce('',b,c) = %q, want b", got)
	}
}

func TestCoalesceAllEmpty(t *testing.T) {
	if got := coalesce("", "", ""); got != "" {
		t.Errorf("coalesce('','','') = %q, want ''", got)
	}
}

// ──────────────────────────────────────────────
// defaultTocLabel
// ──────────────────────────────────────────────

func TestDefaultTocLabelDe(t *testing.T) {
	if got := defaultTocLabel("de"); got != "Inhaltsverzeichnis" {
		t.Errorf("defaultTocLabel(de) = %q", got)
	}
}

func TestDefaultTocLabelEn(t *testing.T) {
	if got := defaultTocLabel("en"); got != "Table of Contents" {
		t.Errorf("defaultTocLabel(en) = %q", got)
	}
}

func TestDefaultTocLabelUnknown(t *testing.T) {
	if got := defaultTocLabel("fr"); got != "Inhaltsverzeichnis" {
		t.Errorf("defaultTocLabel(fr) = %q, want German default", got)
	}
}

// ──────────────────────────────────────────────
// envInt
// ──────────────────────────────────────────────

func TestEnvIntDefault(t *testing.T) {
	if got := envInt("NONEXISTENT_VAR_12345", 42); got != 42 {
		t.Errorf("envInt with missing var = %d, want 42", got)
	}
}

func TestEnvIntParsed(t *testing.T) {
	t.Setenv("TEST_ENV_INT", "99")
	if got := envInt("TEST_ENV_INT", 0); got != 99 {
		t.Errorf("envInt = %d, want 99", got)
	}
}

func TestEnvIntInvalid(t *testing.T) {
	t.Setenv("TEST_ENV_INT_BAD", "notanumber")
	if got := envInt("TEST_ENV_INT_BAD", 7); got != 7 {
		t.Errorf("envInt invalid = %d, want 7", got)
	}
}

// ──────────────────────────────────────────────
// toInt
// ──────────────────────────────────────────────

func TestToIntFloat64(t *testing.T) {
	got, err := toInt(float64(5))
	if err != nil || got != 5 {
		t.Errorf("toInt(float64(5)) = %d, %v", got, err)
	}
}

func TestToIntInt(t *testing.T) {
	got, err := toInt(int(3))
	if err != nil || got != 3 {
		t.Errorf("toInt(int(3)) = %d, %v", got, err)
	}
}

func TestToIntJSONNumber(t *testing.T) {
	n := json.Number("42")
	got, err := toInt(n)
	if err != nil || got != 42 {
		t.Errorf("toInt(json.Number) = %d, %v", got, err)
	}
}

func TestToIntInvalid(t *testing.T) {
	_, err := toInt("hello")
	if err == nil {
		t.Error("expected error for string input")
	}
}

// ──────────────────────────────────────────────
// Error types
// ──────────────────────────────────────────────

func TestErrorTypes(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{&TocError{Message: "toc"}, "toc"},
		{&NoPdfsFoundError{Message: "no pdfs"}, "no pdfs"},
		{&InvalidJsonError{Message: "bad json"}, "bad json"},
		{&TypstCompilationError{Message: "typst fail"}, "typst fail"},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("Error() = %q, want %q", got, tt.want)
		}
	}
}

// ──────────────────────────────────────────────
// addTocLinks
// ──────────────────────────────────────────────

func TestAddTocLinksEmpty(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "input.pdf")
	outPath := filepath.Join(dir, "output.pdf")
	createTestPDF(t, inPath, 3)

	// No entries — should just copy
	err := addTocLinks(inPath, outPath, nil, 1, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	// Output file should exist and have same page count
	pages, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 3 {
		t.Errorf("pages = %d, want 3", pages)
	}
}

func TestAddTocLinksWithEntries(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "input.pdf")
	outPath := filepath.Join(dir, "output.pdf")
	createTestPDF(t, inPath, 5)

	entries := []TocEntry{
		{Title: "Chapter 1", FinalPage: 2},
		{Title: "Chapter 2", FinalPage: 4},
	}
	err := addTocLinks(inPath, outPath, entries, 1, true, true, true)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 5 {
		t.Errorf("pages = %d, want 5", pages)
	}
}

// ──────────────────────────────────────────────
// distributeOutline
// ──────────────────────────────────────────────

func TestDistributeOutlineEmpty(t *testing.T) {
	entries := []TocEntry{{Title: "A", Page: 1, Pages: 5}}
	distributeOutline(nil, entries)
	if len(entries[0].SourceOutline) != 0 {
		t.Error("expected empty outline for nil input")
	}
}

func TestDistributeOutlineAssigns(t *testing.T) {
	entries := []TocEntry{
		{Title: "Part 1", Page: 1, Pages: 5},
		{Title: "Part 2", Page: 6, Pages: 5},
	}
	outline := []OutlineItem{
		{Title: "Intro", Page: 0},     // page 0 → Part 1 (starts at 0)
		{Title: "Section A", Page: 3}, // page 3 → Part 1
		{Title: "Section B", Page: 5}, // page 5 → Part 2 (starts at 5)
		{Title: "Section C", Page: 8}, // page 8 → Part 2
	}

	distributeOutline(outline, entries)

	if len(entries[0].SourceOutline) != 2 {
		t.Errorf("Part 1 outline items = %d, want 2", len(entries[0].SourceOutline))
	}
	if len(entries[1].SourceOutline) != 2 {
		t.Errorf("Part 2 outline items = %d, want 2", len(entries[1].SourceOutline))
	}
	// Check relative page numbers
	if entries[0].SourceOutline[0].Page != 0 {
		t.Errorf("Part 1 item 0 page = %d, want 0", entries[0].SourceOutline[0].Page)
	}
	if entries[0].SourceOutline[1].Page != 3 {
		t.Errorf("Part 1 item 1 page = %d, want 3", entries[0].SourceOutline[1].Page)
	}
	if entries[1].SourceOutline[0].Page != 0 {
		t.Errorf("Part 2 item 0 page = %d, want 0", entries[1].SourceOutline[0].Page)
	}
}

func TestDistributeOutlineWithChildren(t *testing.T) {
	entries := []TocEntry{
		{Title: "Part 1", Page: 1, Pages: 10},
	}
	outline := []OutlineItem{
		{
			Title: "Parent",
			Page:  0,
			Children: []OutlineItem{
				{Title: "Child 1", Page: 2},
				{Title: "Child 2", Page: 5},
			},
		},
	}

	distributeOutline(outline, entries)

	// Parent + 2 children should all land in Part 1
	if len(entries[0].SourceOutline) != 3 {
		t.Errorf("Part 1 outline items = %d, want 3", len(entries[0].SourceOutline))
	}
}

// ──────────────────────────────────────────────
// outlineItemsToBookmarks
// ──────────────────────────────────────────────

func TestOutlineItemsToBookmarksNil(t *testing.T) {
	result := outlineItemsToBookmarks(nil, 0)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestOutlineItemsToBookmarks(t *testing.T) {
	items := []OutlineItem{
		{Title: "A", Page: 0},
		{Title: "B", Page: 3, Children: []OutlineItem{
			{Title: "B1", Page: 4},
		}},
	}
	bms := outlineItemsToBookmarks(items, 5)

	if len(bms) != 2 {
		t.Fatalf("bookmarks = %d, want 2", len(bms))
	}
	if bms[0].PageFrom != 6 { // 5 + 0 + 1
		t.Errorf("bms[0].PageFrom = %d, want 6", bms[0].PageFrom)
	}
	if bms[1].PageFrom != 9 { // 5 + 3 + 1
		t.Errorf("bms[1].PageFrom = %d, want 9", bms[1].PageFrom)
	}
	if len(bms[1].Kids) != 1 {
		t.Fatalf("bms[1].Kids = %d, want 1", len(bms[1].Kids))
	}
	if bms[1].Kids[0].PageFrom != 10 { // 5 + 4 + 1
		t.Errorf("bms[1].Kids[0].PageFrom = %d, want 10", bms[1].Kids[0].PageFrom)
	}
}

// ──────────────────────────────────────────────
// generateTypst
// ──────────────────────────────────────────────

func TestGenerateTypst(t *testing.T) {
	dir := t.TempDir()

	// Create template file
	templatePath := filepath.Join(dir, "toc_template.typ")
	origTemplate := TemplatePath
	TemplatePath = templatePath
	t.Cleanup(func() { TemplatePath = origTemplate })

	templateContent := `// SECTION_LINE_DEF_PLACEHOLDER
// FONT_PLACEHOLDER
// SECTION_LINE_TOP_PLACEHOLDER
// TITLE_SUBTITLE_PLACEHOLDER
// ENTRY_FUNCTION_PLACEHOLDER
// ENTRIES_PLACEHOLDER
// SECTION_LINE_BOTTOM_PLACEHOLDER`
	os.WriteFile(templatePath, []byte(templateContent), 0o644)

	entries := []TocEntry{
		{Title: "Chapter One", Page: 1, Pages: 5},
		{Title: "Chapter Two", Page: 6, Pages: 3},
	}

	outputTyp := filepath.Join(dir, "toc.typ")
	err := generateTypst(entries, 2, outputTyp, GenerateOpts{
		Title:      "My Document",
		Subtitle:   "A Subtitle",
		Font:       "Roboto",
		LineColor:  "#FF0000",
		DotLeaders: true,
		ShowLines:  true,
		JSONMode:   false,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outputTyp)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Check font was set
	if !strings.Contains(content, `font: "Roboto"`) {
		t.Error("expected Roboto font in output")
	}
	// Check color
	if !strings.Contains(content, `#FF0000`) {
		t.Error("expected #FF0000 color in output")
	}
	// Check title
	if !strings.Contains(content, "My Document") {
		t.Error("expected title in output")
	}
	// Check subtitle
	if !strings.Contains(content, "A Subtitle") {
		t.Error("expected subtitle in output")
	}
	// Check dot leaders
	if !strings.Contains(content, "repeat") {
		t.Error("expected dot leaders in output")
	}
	// Check section lines
	if !strings.Contains(content, "#section-line()") {
		t.Error("expected section lines in output")
	}
	// Check entries with correct page offsets (tocOffset=2, so page 3 and 8)
	if !strings.Contains(content, `"3"`) {
		t.Error("expected page 3 for first entry (tocOffset=2, start=3)")
	}
}

func TestGenerateTypstJSONMode(t *testing.T) {
	dir := t.TempDir()

	templatePath := filepath.Join(dir, "toc_template.typ")
	origTemplate := TemplatePath
	TemplatePath = templatePath
	t.Cleanup(func() { TemplatePath = origTemplate })

	templateContent := `// SECTION_LINE_DEF_PLACEHOLDER
// FONT_PLACEHOLDER
// SECTION_LINE_TOP_PLACEHOLDER
// TITLE_SUBTITLE_PLACEHOLDER
// ENTRY_FUNCTION_PLACEHOLDER
// ENTRIES_PLACEHOLDER
// SECTION_LINE_BOTTOM_PLACEHOLDER`
	os.WriteFile(templatePath, []byte(templateContent), 0o644)

	entries := []TocEntry{
		{Title: "Section A", Page: 5, Pages: 10},
	}

	outputTyp := filepath.Join(dir, "toc.typ")
	err := generateTypst(entries, 1, outputTyp, GenerateOpts{
		JSONMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(outputTyp)
	content := string(data)
	// JSON mode: displayPage = entry.Page + tocOffset = 5 + 1 = 6
	if !strings.Contains(content, `"6"`) {
		t.Error("expected page 6 for JSON mode entry")
	}
}

func TestGenerateTypstNoLines(t *testing.T) {
	dir := t.TempDir()

	templatePath := filepath.Join(dir, "toc_template.typ")
	origTemplate := TemplatePath
	TemplatePath = templatePath
	t.Cleanup(func() { TemplatePath = origTemplate })

	templateContent := `// SECTION_LINE_DEF_PLACEHOLDER
// FONT_PLACEHOLDER
// SECTION_LINE_TOP_PLACEHOLDER
// TITLE_SUBTITLE_PLACEHOLDER
// ENTRY_FUNCTION_PLACEHOLDER
// ENTRIES_PLACEHOLDER
// SECTION_LINE_BOTTOM_PLACEHOLDER`
	os.WriteFile(templatePath, []byte(templateContent), 0o644)

	outputTyp := filepath.Join(dir, "toc.typ")
	err := generateTypst(nil, 1, outputTyp, GenerateOpts{
		ShowLines:  false,
		DotLeaders: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(outputTyp)
	content := string(data)
	if strings.Contains(content, "#section-line()") {
		t.Error("expected no section lines when ShowLines=false")
	}
	if strings.Contains(content, "repeat") {
		t.Error("expected no dot leaders when DotLeaders=false")
	}
}

// ──────────────────────────────────────────────
// loadJSONEntries with non-map entry
// ──────────────────────────────────────────────

func TestLoadJSONEntriesNonMapItem(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDF(t, pdfPath, 5)

	data := []interface{}{
		"not a map",
	}
	_, err := loadJSONEntries(data, pdfPath)
	if err == nil {
		t.Fatal("expected error for non-map entry")
	}
	if !isInvalidJSON(err) {
		t.Errorf("expected InvalidJsonError, got %T", err)
	}
}

func TestLoadJSONEntriesNonStringTitle(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	createTestPDF(t, pdfPath, 5)

	data := []interface{}{
		map[string]interface{}{"title": 123, "page": float64(1)},
	}
	_, err := loadJSONEntries(data, pdfPath)
	if err == nil {
		t.Fatal("expected error for non-string title")
	}
}

// ──────────────────────────────────────────────
// helper
// ──────────────────────────────────────────────

func isInvalidJSON(err error) bool {
	_, ok := err.(*InvalidJsonError)
	return ok
}

// ──────────────────────────────────────────────
// compileTypst (requires typst binary)
// ──────────────────────────────────────────────

func skipIfNoTypst(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not installed, skipping")
	}
}

func setupTypstTemplate(t *testing.T) (origTemplate string) {
	t.Helper()
	origTemplate = TemplatePath

	// Find actual template
	cwd, _ := os.Getwd()
	templateSrc := filepath.Join(cwd, "templates", "toc_template.typ")
	if _, err := os.Stat(templateSrc); err != nil {
		t.Skipf("template not found: %v", err)
	}
	TemplatePath = templateSrc
	return origTemplate
}

func TestCompileTypst(t *testing.T) {
	skipIfNoTypst(t)
	origTemplate := setupTypstTemplate(t)
	t.Cleanup(func() { TemplatePath = origTemplate })

	entries := []TocEntry{
		{Title: "Chapter 1", Pages: 3},
		{Title: "Chapter 2", Pages: 5},
	}

	workDir := t.TempDir()
	pdfFile, tocPages, err := compileTypst(entries, workDir, GenerateOpts{
		Title:      "Test Document",
		Subtitle:   "Subtitle",
		DotLeaders: true,
		ShowLines:  true,
		LineColor:  "#FF0000",
		JSONMode:   false,
	})
	if err != nil {
		t.Fatal(err)
	}

	if tocPages < 1 {
		t.Errorf("tocPages = %d, want >= 1", tocPages)
	}
	if _, err := os.Stat(pdfFile); err != nil {
		t.Errorf("compiled PDF not found: %v", err)
	}

	// Verify FinalPage was computed
	if entries[0].FinalPage != tocPages+1 {
		t.Errorf("entry[0].FinalPage = %d, want %d", entries[0].FinalPage, tocPages+1)
	}
	if entries[1].FinalPage != tocPages+1+3 {
		t.Errorf("entry[1].FinalPage = %d, want %d", entries[1].FinalPage, tocPages+1+3)
	}
}

func TestCompileTypstJSONMode(t *testing.T) {
	skipIfNoTypst(t)
	origTemplate := setupTypstTemplate(t)
	t.Cleanup(func() { TemplatePath = origTemplate })

	entries := []TocEntry{
		{Title: "Section A", Page: 5, Pages: 10},
		{Title: "Section B", Page: 15, Pages: 5},
	}

	workDir := t.TempDir()
	_, tocPages, err := compileTypst(entries, workDir, GenerateOpts{
		JSONMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// JSON mode: FinalPage = entry.Page + tocPages
	if entries[0].FinalPage != 5+tocPages {
		t.Errorf("entry[0].FinalPage = %d, want %d", entries[0].FinalPage, 5+tocPages)
	}
}

// ──────────────────────────────────────────────
// processFolderMode full (requires typst)
// ──────────────────────────────────────────────

func TestProcessFolderModeFull(t *testing.T) {
	skipIfNoTypst(t)
	origTemplate := setupTypstTemplate(t)
	t.Cleanup(func() { TemplatePath = origTemplate })

	dir := t.TempDir()
	pdf1 := filepath.Join(dir, "doc1.pdf")
	pdf2 := filepath.Join(dir, "doc2.pdf")
	outPath := filepath.Join(dir, "output.pdf")
	createTestPDF(t, pdf1, 2)
	createTestPDF(t, pdf2, 3)

	filename, err := processFolderMode(FolderModeOpts{
		PDFFiles: []PDFFile{
			{Path: pdf1, OriginalName: "doc1.pdf"},
			{Path: pdf2, OriginalName: "doc2.pdf"},
		},
		Title:      "Full TOC Test",
		Subtitle:   "Subtitle Here",
		Font:       "IBM Plex Sans",
		LineColor:  "#2563EB",
		DotLeaders: true,
		ShowLines:  true,
		MergeOnly:  false,
		OutputPath: outPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if filename != "Full TOC Test.pdf" {
		t.Errorf("filename = %q, want 'Full TOC Test.pdf'", filename)
	}

	pages, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	// Should have TOC page(s) + 2 + 3 = 6+ pages
	if pages < 6 {
		t.Errorf("output pages = %d, want >= 6", pages)
	}
}

func TestProcessFolderModeNoPDFs(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "output.pdf")

	_, err := processFolderMode(FolderModeOpts{
		PDFFiles:   nil,
		OutputPath: outPath,
	})
	if err == nil {
		t.Fatal("expected error for no PDFs")
	}
	var noPdfs *NoPdfsFoundError
	if !errors.As(err, &noPdfs) {
		t.Errorf("expected NoPdfsFoundError, got %T: %v", err, err)
	}
}

func TestProcessFolderModeDefaultTitle(t *testing.T) {
	skipIfNoTypst(t)
	origTemplate := setupTypstTemplate(t)
	t.Cleanup(func() { TemplatePath = origTemplate })

	dir := t.TempDir()
	pdf1 := filepath.Join(dir, "doc1.pdf")
	outPath := filepath.Join(dir, "output.pdf")
	createTestPDF(t, pdf1, 2)

	filename, err := processFolderMode(FolderModeOpts{
		PDFFiles: []PDFFile{
			{Path: pdf1, OriginalName: "doc1.pdf"},
		},
		Lang:       "en",
		MergeOnly:  false,
		OutputPath: outPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if filename != "Table of Contents.pdf" {
		t.Errorf("filename = %q, want 'Table of Contents.pdf'", filename)
	}
}

// ──────────────────────────────────────────────
// processJSONMode (requires typst)
// ──────────────────────────────────────────────

func TestProcessJSONMode(t *testing.T) {
	skipIfNoTypst(t)
	origTemplate := setupTypstTemplate(t)
	t.Cleanup(func() { TemplatePath = origTemplate })

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	outPath := filepath.Join(dir, "output.pdf")
	createTestPDF(t, pdfPath, 10)

	jsonData := []byte(`[
		{"title": "Chapter 1", "page": 1, "pages": 5},
		{"title": "Chapter 2", "page": 6, "pages": 5}
	]`)

	filename, err := processJSONMode(JSONModeOpts{
		PDFPath:    pdfPath,
		JSONData:   jsonData,
		Title:      "JSON TOC",
		Subtitle:   "Test",
		Font:       "Roboto",
		LineColor:  "#059669",
		Lang:       "de",
		DotLeaders: true,
		ShowLines:  true,
		OutputPath: outPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if filename != "JSON TOC.pdf" {
		t.Errorf("filename = %q, want 'JSON TOC.pdf'", filename)
	}

	pages, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	// Should have TOC page(s) + 10 original pages
	if pages < 11 {
		t.Errorf("output pages = %d, want >= 11", pages)
	}
}

func TestProcessJSONModeInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	outPath := filepath.Join(dir, "output.pdf")
	createTestPDF(t, pdfPath, 5)

	_, err := processJSONMode(JSONModeOpts{
		PDFPath:    pdfPath,
		JSONData:   []byte(`{invalid`),
		OutputPath: outPath,
	})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestProcessJSONModeNotArray(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "test.pdf")
	outPath := filepath.Join(dir, "output.pdf")
	createTestPDF(t, pdfPath, 5)

	_, err := processJSONMode(JSONModeOpts{
		PDFPath:    pdfPath,
		JSONData:   []byte(`{"key": "value"}`),
		OutputPath: outPath,
	})
	if err == nil {
		t.Fatal("expected error for non-array JSON")
	}
}

func TestProcessJSONModeDefaultTitle(t *testing.T) {
	skipIfNoTypst(t)
	origTemplate := setupTypstTemplate(t)
	t.Cleanup(func() { TemplatePath = origTemplate })

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "my_document.pdf")
	outPath := filepath.Join(dir, "output.pdf")
	createTestPDF(t, pdfPath, 5)

	jsonData := []byte(`[{"title": "A", "page": 1}]`)

	filename, err := processJSONMode(JSONModeOpts{
		PDFPath:    pdfPath,
		JSONData:   jsonData,
		OutputPath: outPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Title derived from PDF filename: "my_document" → "My Document" → "My Document.pdf"
	if filename != "My Document.pdf" {
		t.Errorf("filename = %q, want 'My Document.pdf'", filename)
	}
}

// ──────────────────────────────────────────────
// capitalize edge case
// ──────────────────────────────────────────────

func TestCapitalizeEmpty(t *testing.T) {
	if got := capitalize(""); got != "" {
		t.Errorf("capitalize('') = %q", got)
	}
}

// pdfcpu Bookmark imported for test usage
var _ = pdfcpu.Bookmark{}

// ensure json imported for test usage
var _ = json.Unmarshal
