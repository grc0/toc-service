package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ──────────────────────────────────────────────
// Errors
// ──────────────────────────────────────────────

type TocError struct{ Message string }
type NoPdfsFoundError struct{ Message string }
type InvalidJsonError struct{ Message string }
type TypstCompilationError struct{ Message string }

func (e *TocError) Error() string             { return e.Message }
func (e *NoPdfsFoundError) Error() string      { return e.Message }
func (e *InvalidJsonError) Error() string      { return e.Message }
func (e *TypstCompilationError) Error() string { return e.Message }

// ──────────────────────────────────────────────
// Configuration
// ──────────────────────────────────────────────

const (
	DefaultFont      = "IBM Plex Sans"
	DefaultLineColor = "#1A2980"
	DefaultBgColor   = "#FFFFFF"
	DefaultFontColor = "#000000"
)

var (
	TypstTimeoutSeconds = envInt("TYPST_TIMEOUT_SECONDS", 30)
	TemplatePath        = resolveTemplatePath()

	AllowedFonts = map[string]string{
		"IBM Plex Sans": "IBM Plex Sans",
		"Roboto":        "Roboto",
		"Inter":         "Inter",
	}

	DefaultTocLabel = map[string]string{
		"de": "Inhaltsverzeichnis",
		"en": "Table of Contents",
	}

	LineColors = map[string]string{
		"#1A2980": "Navy",
		"#000000": "Black",
		"#2563EB": "Blue",
		"#0891B2": "Teal",
		"#059669": "Green",
		"#65A30D": "Lime",
		"#CA8A04": "Gold",
		"#EA580C": "Orange",
		"#DC2626": "Red",
		"#9333EA": "Purple",
		"#E84466": "Pink",
		"#4B5563": "Gray",
		"#FFFFFF": "White",
	}

	FontColors = map[string]string{
		"#000000": "Black",
		"#FFFFFF": "White",
		"#1A2980": "Navy",
		"#2563EB": "Blue",
		"#0891B2": "Teal",
		"#059669": "Green",
		"#DC2626": "Red",
		"#9333EA": "Purple",
		"#4B5563": "Gray",
	}

	BgColors = map[string]string{
		"#FFFFFF": "White",
		"#F3F4F6": "Light Gray",
		"#FFFBEB": "Cream",
		"#EFF6FF": "Light Blue",
		"#F0FDF4": "Light Green",
		"#FDF2F8": "Light Pink",
		"#F5F3FF": "Light Purple",
		"#ECFDF5": "Mint",
		"#000000": "Black",
	}
)

// ──────────────────────────────────────────────
// Data types
// ──────────────────────────────────────────────

type TocEntry struct {
	Path          string
	Title         string
	Page          int // 1-based starting page in source PDF (JSON mode)
	Pages         int // number of pages
	FinalPage     int // 1-based page in merged output PDF
	SourceOutline []OutlineItem
}

type OutlineItem struct {
	Title    string
	Page     int // 0-based page relative to source PDF
	Children []OutlineItem
}

type PDFFile struct {
	Path         string
	OriginalName string
	Title        string // optional override; empty → derive from filename
}

type GenerateOpts struct {
	Title      string
	Subtitle   string
	Font       string
	LineColor  string
	BgColor    string
	FontColor  string
	DotLeaders bool
	ShowLines  bool
	JSONMode   bool
}

type FolderModeOpts struct {
	PDFFiles   []PDFFile
	Title      string
	Subtitle   string
	Font       string
	LineColor  string
	BgColor    string
	FontColor  string
	Lang       string
	DotLeaders bool
	ShowLines  bool
	MergeOnly  bool
	OutputPath string
}

type JSONModeOpts struct {
	PDFPath    string
	JSONData   []byte
	Title      string
	Subtitle   string
	Font       string
	LineColor  string
	BgColor    string
	FontColor  string
	Lang       string
	DotLeaders bool
	ShowLines  bool
	OutputPath string
}

// ──────────────────────────────────────────────
// Helper functions
// ──────────────────────────────────────────────

func resolveTemplatePath() string {
	if p := os.Getenv("TEMPLATE_PATH"); p != "" {
		return p
	}
	// Try relative to executable first (for Docker / installed binary)
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "templates", "toc_template.typ")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Fallback: relative to working directory
	return filepath.Join("templates", "toc_template.typ")
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
		return n
	}
	return def
}

func defaultTocLabel(lang string) string {
	if label, ok := DefaultTocLabel[lang]; ok {
		return label
	}
	return DefaultTocLabel["de"]
}

var colorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func validateColor(color string) string {
	color = strings.TrimSpace(color)
	if !strings.HasPrefix(color, "#") {
		color = "#" + color
	}
	if colorRe.MatchString(color) {
		return strings.ToUpper(color)
	}
	return DefaultLineColor
}

func escapeTypst(text string) string {
	text = strings.ReplaceAll(text, `\`, `\\`)
	text = strings.ReplaceAll(text, `"`, `\"`)
	text = strings.ReplaceAll(text, `#`, `\#`)
	text = strings.ReplaceAll(text, `[`, `\[`)
	text = strings.ReplaceAll(text, `]`, `\]`)
	return text
}

const maxEntryTitleLen = 80

func truncateEntry(title string) string {
	runes := []rune(title)
	if len(runes) <= maxEntryTitleLen {
		return title
	}
	return string(runes[:maxEntryTitleLen]) + "..."
}

var lowercaseWords = map[string]bool{
	"und": true, "oder": true, "für": true, "von": true,
	"mit": true, "im": true, "am": true, "an": true,
}

func folderToTitle(folderName string) string {
	title := strings.ReplaceAll(folderName, "_", " ")
	title = strings.ReplaceAll(title, "-", " ")
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
	title = strings.TrimSpace(title)

	words := strings.Fields(title)
	if len(words) == 0 {
		return "Inhaltsverzeichnis"
	}

	result := make([]string, len(words))
	for i, word := range words {
		if i == 0 || !lowercaseWords[strings.ToLower(word)] {
			result[i] = capitalize(word)
		} else {
			result[i] = strings.ToLower(word)
		}
	}
	title = strings.Join(result, " ")
	title = regexp.MustCompile(`\bAus und\b`).ReplaceAllString(title, "Aus- und")
	return title
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	for i := 1; i < len(runes); i++ {
		runes[i] = []rune(strings.ToLower(string(runes[i])))[0]
	}
	return string(runes)
}

var leadingNumbersRe = regexp.MustCompile(`^\d+[\s\-_.]+`)

func cleanTitle(filename string) string {
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " – ")
	name = leadingNumbersRe.ReplaceAllString(name, "")
	return strings.TrimSpace(name)
}

var unsafeFilenameRe = regexp.MustCompile(`[<>:"/\\|?*]`)

func titleToFilename(title string) string {
	name := unsafeFilenameRe.ReplaceAllString(title, "")
	name = strings.TrimSpace(name)
	if name == "" {
		return "ausgabe.pdf"
	}
	return name + ".pdf"
}

func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ──────────────────────────────────────────────
// Step 1: Read PDFs
// ──────────────────────────────────────────────

func collectPDFs(inputDir string) ([]TocEntry, error) {
	matches, err := filepath.Glob(filepath.Join(inputDir, "*.pdf"))
	if err != nil {
		return nil, err
	}
	// Sort reverse by name (matching Python's sorted(..., reverse=True))
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}

	var entries []TocEntry
	for _, pdfFile := range matches {
		pages, err := api.PageCountFile(pdfFile)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filepath.Base(pdfFile), err)
		}
		entries = append(entries, TocEntry{
			Path:  pdfFile,
			Title: cleanTitle(filepath.Base(pdfFile)),
			Pages: pages,
		})
	}

	if len(entries) == 0 {
		return nil, &NoPdfsFoundError{Message: fmt.Sprintf("Keine PDF-Dateien in '%s' gefunden.", inputDir)}
	}
	return entries, nil
}

// ──────────────────────────────────────────────
// Step 1b: Load JSON entries (Mode B)
// ──────────────────────────────────────────────

func loadJSONEntries(jsonData []interface{}, pdfPath string) ([]TocEntry, error) {
	if len(jsonData) == 0 {
		return nil, &InvalidJsonError{Message: "JSON muss eine nicht-leere Liste sein."}
	}

	totalPages, err := api.PageCountFile(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("reading PDF: %w", err)
	}

	entries := make([]TocEntry, 0, len(jsonData))
	for i, item := range jsonData {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, &InvalidJsonError{
				Message: fmt.Sprintf("Eintrag %d ist ungültig.", i+1),
			}
		}

		titleRaw, hasTitle := m["title"]
		pageRaw, hasPage := m["page"]
		if !hasTitle || !hasPage {
			return nil, &InvalidJsonError{
				Message: fmt.Sprintf("Eintrag %d braucht die Felder 'title' und 'page'.", i+1),
			}
		}

		title, ok := titleRaw.(string)
		if !ok {
			return nil, &InvalidJsonError{
				Message: fmt.Sprintf("Eintrag %d: 'title' muss ein String sein.", i+1),
			}
		}

		page, err := toInt(pageRaw)
		if err != nil {
			return nil, &InvalidJsonError{
				Message: fmt.Sprintf("Eintrag %d: 'page' muss eine Ganzzahl sein, erhalten: %v", i+1, pageRaw),
			}
		}

		if page < 1 || page > totalPages {
			return nil, &InvalidJsonError{
				Message: fmt.Sprintf("Eintrag %d: Seite %d liegt außerhalb des PDFs (1–%d).", i+1, page, totalPages),
			}
		}

		var pages int
		if pagesRaw, ok := m["pages"]; ok {
			pages, err = toInt(pagesRaw)
			if err != nil {
				return nil, &InvalidJsonError{
					Message: fmt.Sprintf("Eintrag %d: 'pages' muss eine Ganzzahl sein, erhalten: %v", i+1, pagesRaw),
				}
			}
		} else if i+1 < len(jsonData) {
			nextItem, ok := jsonData[i+1].(map[string]interface{})
			if !ok {
				return nil, &InvalidJsonError{
					Message: fmt.Sprintf("Eintrag %d ist ungültig.", i+2),
				}
			}
			nextPage, err := toInt(nextItem["page"])
			if err != nil {
				return nil, &InvalidJsonError{
					Message: fmt.Sprintf("Eintrag %d: 'page' muss eine Ganzzahl sein, erhalten: %v", i+2, nextItem["page"]),
				}
			}
			pages = nextPage - page
		} else {
			pages = totalPages - page + 1
		}

		entries = append(entries, TocEntry{
			Title: title,
			Page:  page,
			Pages: pages,
		})
	}

	return entries, nil
}

func toInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case float64:
		return int(val), nil
	case int:
		return val, nil
	case json.Number:
		n, err := val.Int64()
		return int(n), err
	default:
		return 0, fmt.Errorf("not a number: %v", v)
	}
}

// ──────────────────────────────────────────────
// Step 2: Generate Typst file
// ──────────────────────────────────────────────

func generateTypst(entries []TocEntry, tocOffset int, outputTyp string, opts GenerateOpts) error {
	templateBytes, err := os.ReadFile(TemplatePath)
	if err != nil {
		return fmt.Errorf("reading template: %w", err)
	}
	content := string(templateBytes)

	// Font settings
	fontName := DefaultFont
	if opts.Font != "" {
		if f, ok := AllowedFonts[opts.Font]; ok {
			fontName = f
		}
	}
	fontColorHex := DefaultFontColor
	if opts.FontColor != "" {
		fontColorHex = validateColor(opts.FontColor)
	}
	var fontTypst string
	if fontColorHex != "" && fontColorHex != "#000000" {
		fontTypst = fmt.Sprintf(`#set text(font: "%s", size: 11pt, lang: "de", fill: rgb("%s"))`, fontName, fontColorHex)
	} else {
		fontTypst = fmt.Sprintf(`#set text(font: "%s", size: 11pt, lang: "de")`, fontName)
	}
	content = strings.Replace(content, "// FONT_PLACEHOLDER", fontTypst, 1)

	// Section line definition with color
	colorHex := DefaultLineColor
	if opts.LineColor != "" {
		colorHex = validateColor(opts.LineColor)
	}
	sectionLineDef := fmt.Sprintf(
		`#let section-line() = block(width: 100%%, height: 0.1cm, fill: rgb("%s"), radius: 0.5pt)`,
		colorHex,
	)
	content = strings.Replace(content, "// SECTION_LINE_DEF_PLACEHOLDER", sectionLineDef, 1)

	// Background color
	bgColorHex := DefaultBgColor
	if opts.BgColor != "" {
		bgColorHex = validateColor(opts.BgColor)
	}
	if bgColorHex != "" && bgColorHex != "#FFFFFF" {
		bgFill := fmt.Sprintf(`fill: rgb("%s"),`, bgColorHex)
		content = strings.Replace(content, "// BG_COLOR_PLACEHOLDER", bgFill, 1)
	} else {
		content = strings.Replace(content, "// BG_COLOR_PLACEHOLDER", "", 1)
	}

	// Entry function (with or without dot leaders)
	filler := ""
	if opts.DotLeaders {
		filler = `#repeat[#text(fill: luma(140), size: 9pt)[ .]]`
	}
	entryFn := "#let entry(title, page_num) = {\n" +
		"  box(width: 100%)[\n" +
		"    #text(size: 9pt, weight: \"regular\")[#title]\n" +
		fmt.Sprintf("    #box(width: 1fr, baseline: 40%%)[%s]\n", filler) +
		"    #text(size: 9pt, weight: \"regular\")[#page_num]\n" +
		"  ]\n" +
		"  v(0.65em)\n" +
		"}"
	content = strings.Replace(content, "// ENTRY_FUNCTION_PLACEHOLDER", entryFn, 1)

	// Section lines (top and bottom bars)
	if opts.ShowLines {
		content = strings.Replace(content, "// SECTION_LINE_TOP_PLACEHOLDER", "#section-line()", 1)
		content = strings.Replace(content, "// SECTION_LINE_BOTTOM_PLACEHOLDER", "#section-line()", 1)
	} else {
		content = strings.Replace(content, "// SECTION_LINE_TOP_PLACEHOLDER", "", 1)
		content = strings.Replace(content, "// SECTION_LINE_BOTTOM_PLACEHOLDER", "", 1)
	}

	// Title + subtitle block
	var headerParts []string
	if opts.Title != "" {
		safeTitle := escapeTypst(opts.Title)
		headerParts = append(headerParts,
			fmt.Sprintf("#v(0.4cm)\n#text(size: 1.4em, weight: \"bold\")[%s]", safeTitle))
	}
	if opts.Subtitle != "" {
		safeSubtitle := escapeTypst(opts.Subtitle)
		subtitleFill := "luma(80)"
		if fontColorHex != "" && fontColorHex != "#000000" {
			subtitleFill = fmt.Sprintf(`rgb("%s").lighten(30%%)`, fontColorHex)
		}
		headerParts = append(headerParts,
			fmt.Sprintf("#v(0.15cm)\n#text(size: 1.1em, weight: \"regular\", fill: %s)[%s]", subtitleFill, safeSubtitle))
	}
	content = strings.Replace(content, "// TITLE_SUBTITLE_PLACEHOLDER", strings.Join(headerParts, "\n"), 1)

	// TOC entries
	var lines []string
	if opts.JSONMode {
		for _, e := range entries {
			displayPage := e.Page + tocOffset
			safeTitle := escapeTypst(truncateEntry(e.Title))
			lines = append(lines, fmt.Sprintf(`#entry("%s", "%d")`, safeTitle, displayPage))
		}
	} else {
		currentPage := tocOffset + 1
		for _, e := range entries {
			safeTitle := escapeTypst(truncateEntry(e.Title))
			lines = append(lines, fmt.Sprintf(`#entry("%s", "%d")`, safeTitle, currentPage))
			currentPage += e.Pages
		}
	}
	content = strings.Replace(content, "// ENTRIES_PLACEHOLDER", strings.Join(lines, "\n"), 1)

	return os.WriteFile(outputTyp, []byte(content), 0o644)
}

// ──────────────────────────────────────────────
// Step 3: Compile Typst (with iteration)
// ──────────────────────────────────────────────

func compileTypst(entries []TocEntry, workDir string, opts GenerateOpts) (string, int, error) {
	typFile := filepath.Join(workDir, "toc.typ")
	pdfFile := filepath.Join(workDir, "toc.pdf")

	tocPagesGuess := 1

	for iteration := 0; iteration < 5; iteration++ {
		if err := generateTypst(entries, tocPagesGuess, typFile, opts); err != nil {
			return "", 0, err
		}

		cmd := exec.Command("typst", "compile", typFile, pdfFile)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if ctx := cmd.ProcessState; ctx != nil && !ctx.Exited() {
				return "", 0, &TypstCompilationError{
					Message: fmt.Sprintf("Typst-Kompilierung abgebrochen: Timeout nach %d Sekunden.", TypstTimeoutSeconds),
				}
			}
			return "", 0, &TypstCompilationError{
				Message: fmt.Sprintf("Typst-Kompilierung fehlgeschlagen:\n%s", string(output)),
			}
		}

		actualPages, err := api.PageCountFile(pdfFile)
		if err != nil {
			return "", 0, fmt.Errorf("reading compiled PDF: %w", err)
		}

		if actualPages == tocPagesGuess {
			break
		}
		tocPagesGuess = actualPages
	}

	// Compute final page mapping
	if opts.JSONMode {
		for i := range entries {
			entries[i].FinalPage = entries[i].Page + tocPagesGuess
		}
	} else {
		currentPage := tocPagesGuess + 1
		for i := range entries {
			entries[i].FinalPage = currentPage
			currentPage += entries[i].Pages
		}
	}

	return pdfFile, tocPagesGuess, nil
}

// ──────────────────────────────────────────────
// Step 4: Merge PDFs
// ──────────────────────────────────────────────

func mergePDFFiles(inFiles []string, outFile string) error {
	return api.MergeCreateFile(inFiles, outFile, false, nil)
}

// ──────────────────────────────────────────────
// Step 5: Clickable links on TOC page(s)
// ──────────────────────────────────────────────

type titleBlockKey struct {
	showLines  bool
	hasTitle   bool
	hasSubtitle bool
}

var titleBlockOffsets = map[titleBlockKey]float64{
	{true, true, true}:    126.3,
	{true, true, false}:   99.3,
	{true, false, true}:   82.3,
	{true, false, false}:  53.3,
	{false, true, true}:   105.3,
	{false, true, false}:  78.3,
	{false, false, true}:  64.3,
	{false, false, false}: 37.3,
}

func computeTitleBlock(hasTitle, hasSubtitle, showLines bool) float64 {
	return titleBlockOffsets[titleBlockKey{showLines, hasTitle, hasSubtitle}]
}

func addTocLinks(inFile, outFile string, entries []TocEntry, tocPages int,
	hasTitle, hasSubtitle, showLines bool) error {

	if len(entries) == 0 {
		// No entries → just copy the file
		data, err := os.ReadFile(inFile)
		if err != nil {
			return err
		}
		return os.WriteFile(outFile, data, 0o644)
	}

	const (
		pageHeight  = 841.89
		topMargin   = 56.69
		leftMargin  = 70.87
		rightEdge   = 595.28 - 70.87
		lineHeight  = 30.5
		linkShiftUp = 6.0 // shift link rectangles up to center on text
	)
	titleBlock := computeTitleBlock(hasTitle, hasSubtitle, showLines)
	entriesPerPage := int((pageHeight - topMargin*2 - titleBlock) / lineHeight)

	annMap := make(map[int][]model.AnnotationRenderer)

	for i, entry := range entries {
		tocPageIdx := i / entriesPerPage
		entryOnPage := i % entriesPerPage

		if tocPageIdx >= tocPages {
			continue
		}

		yTop := pageHeight - topMargin - titleBlock - float64(entryOnPage)*lineHeight + linkShiftUp
		yBottom := yTop - lineHeight + 6

		destPageNr := entry.FinalPage // 1-based, which pdfcpu expects
		tocPageNr := tocPageIdx + 1   // 1-based

		rect := *types.NewRectangle(leftMargin, yBottom, rightEdge, yTop)
		dest := &model.Destination{
			Typ:    model.DestFit,
			PageNr: destPageNr,
		}
		link := model.NewLinkAnnotation(
			rect,
			0,             // apObjNr
			"",            // contents
			"",            // id
			"",            // modDate
			0,             // flags
			nil,           // borderCol
			dest,          // dest
			"",            // uri
			nil,           // quad
			false,         // border
			0,             // borderWidth
			model.BSSolid, // borderStyle
		)

		annMap[tocPageNr] = append(annMap[tocPageNr], link)
	}

	in, err := os.Open(inFile)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(outFile)
	if err != nil {
		return err
	}
	defer out.Close()

	return api.AddAnnotationsMap(in, out, annMap, nil)
}

// ──────────────────────────────────────────────
// Step 5b: Extract existing bookmarks
// ──────────────────────────────────────────────

func extractOutline(pdfPath string) ([]OutlineItem, error) {
	f, err := os.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	bms, err := api.Bookmarks(f, nil)
	if err != nil {
		// Silently return empty on error (matching Python behavior)
		return nil, nil
	}
	return bookmarksToOutlineItems(bms), nil
}

func bookmarksToOutlineItems(bms []pdfcpu.Bookmark) []OutlineItem {
	if len(bms) == 0 {
		return nil
	}
	items := make([]OutlineItem, 0, len(bms))
	for _, bm := range bms {
		items = append(items, OutlineItem{
			Title:    bm.Title,
			Page:     bm.PageFrom - 1, // pdfcpu is 1-based → 0-based
			Children: bookmarksToOutlineItems(bm.Kids),
		})
	}
	return items
}

func distributeOutline(sourceOutline []OutlineItem, entries []TocEntry) {
	if len(sourceOutline) == 0 {
		return
	}

	var assign func(items []OutlineItem)
	assign = func(items []OutlineItem) {
		for _, item := range items {
			bookmarkPage := item.Page // 0-indexed in source PDF
			for i := range entries {
				start := entries[i].Page - 1 // Convert to 0-indexed
				var end int
				if i+1 < len(entries) {
					end = entries[i+1].Page - 1
				} else {
					end = math.MaxInt32
				}
				if bookmarkPage >= start && bookmarkPage < end {
					entries[i].SourceOutline = append(entries[i].SourceOutline, OutlineItem{
						Title:    item.Title,
						Page:     bookmarkPage - start,
						Children: nil,
					})
					break
				}
			}
			if len(item.Children) > 0 {
				assign(item.Children)
			}
		}
	}
	assign(sourceOutline)
}

// ──────────────────────────────────────────────
// Step 6: PDF bookmarks
// ──────────────────────────────────────────────

func addBookmarks(inFile, outFile string, entries []TocEntry, tocTitle string) error {
	// Build bookmark hierarchy
	var entryBookmarks []pdfcpu.Bookmark
	for _, entry := range entries {
		bm := pdfcpu.Bookmark{
			Title:    entry.Title,
			PageFrom: entry.FinalPage, // 1-based
		}

		// Nest source bookmarks under this entry
		if len(entry.SourceOutline) > 0 {
			bm.Kids = outlineItemsToBookmarks(entry.SourceOutline, entry.FinalPage-1)
		}

		entryBookmarks = append(entryBookmarks, bm)
	}

	topBookmark := pdfcpu.Bookmark{
		Title:    tocTitle,
		PageFrom: 1, // First page
		Bold:     true,
		Kids:     entryBookmarks,
	}

	return api.AddBookmarksFile(inFile, outFile, []pdfcpu.Bookmark{topBookmark}, true, nil)
}

func outlineItemsToBookmarks(items []OutlineItem, pageOffset int) []pdfcpu.Bookmark {
	if len(items) == 0 {
		return nil
	}
	bms := make([]pdfcpu.Bookmark, 0, len(items))
	for _, item := range items {
		pageNr := pageOffset + item.Page + 1 // Convert 0-based to 1-based
		bm := pdfcpu.Bookmark{
			Title:    item.Title,
			PageFrom: pageNr,
		}
		if len(item.Children) > 0 {
			bm.Kids = outlineItemsToBookmarks(item.Children, pageOffset)
		}
		bms = append(bms, bm)
	}
	return bms
}

// ──────────────────────────────────────────────
// Orchestrator: Folder mode (Mode A)
// ──────────────────────────────────────────────

func processFolderMode(opts FolderModeOpts) (string, error) {
	defaultLabel := defaultTocLabel(opts.Lang)
	outputFilename := defaultLabel + ".pdf"
	if opts.Title != "" {
		outputFilename = titleToFilename(opts.Title)
	}

	// Build entries from uploaded PDF files
	entries := make([]TocEntry, 0, len(opts.PDFFiles))
	for _, pf := range opts.PDFFiles {
		pages, err := api.PageCountFile(pf.Path)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", pf.OriginalName, err)
		}
		outline, _ := extractOutline(pf.Path)
		title := strings.TrimSpace(pf.Title)
		if title == "" {
			title = cleanTitle(pf.OriginalName)
		}
		entries = append(entries, TocEntry{
			Path:          pf.Path,
			Title:         title,
			Pages:         pages,
			SourceOutline: outline,
		})
	}

	if len(entries) == 0 {
		return "", &NoPdfsFoundError{Message: "Keine PDF-Dateien hochgeladen."}
	}

	workDir, err := os.MkdirTemp("", "toc-work-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workDir)

	if opts.MergeOnly {
		// Merge-only mode: no visible TOC page
		currentPage := 1
		for i := range entries {
			entries[i].FinalPage = currentPage
			currentPage += entries[i].Pages
		}

		inFiles := make([]string, len(entries))
		for i, e := range entries {
			inFiles[i] = e.Path
		}

		mergedPath := filepath.Join(workDir, "merged.pdf")
		if err := mergePDFFiles(inFiles, mergedPath); err != nil {
			return "", fmt.Errorf("merging PDFs: %w", err)
		}

		bookmarkTitle := coalesce(opts.Title, opts.Subtitle, defaultLabel)
		if err := addBookmarks(mergedPath, opts.OutputPath, entries, bookmarkTitle); err != nil {
			return "", fmt.Errorf("adding bookmarks: %w", err)
		}
		return outputFilename, nil
	}

	// Full TOC mode
	genOpts := GenerateOpts{
		Title:      opts.Title,
		Subtitle:   opts.Subtitle,
		Font:       opts.Font,
		DotLeaders: opts.DotLeaders,
		ShowLines:  opts.ShowLines,
		LineColor:  opts.LineColor,
		BgColor:    opts.BgColor,
		FontColor:  opts.FontColor,
		JSONMode:   false,
	}

	tocPDF, tocPages, err := compileTypst(entries, workDir, genOpts)
	if err != nil {
		return "", err
	}

	// Merge TOC + content PDFs
	inFiles := make([]string, 0, len(entries)+1)
	inFiles = append(inFiles, tocPDF)
	for _, e := range entries {
		inFiles = append(inFiles, e.Path)
	}
	mergedPath := filepath.Join(workDir, "merged.pdf")
	if err := mergePDFFiles(inFiles, mergedPath); err != nil {
		return "", fmt.Errorf("merging PDFs: %w", err)
	}

	// Add TOC links
	annotatedPath := filepath.Join(workDir, "annotated.pdf")
	if err := addTocLinks(mergedPath, annotatedPath, entries, tocPages,
		opts.Title != "", opts.Subtitle != "", opts.ShowLines); err != nil {
		return "", fmt.Errorf("adding TOC links: %w", err)
	}

	// Add bookmarks
	bookmarkTitle := coalesce(opts.Title, opts.Subtitle, defaultLabel)
	if err := addBookmarks(annotatedPath, opts.OutputPath, entries, bookmarkTitle); err != nil {
		return "", fmt.Errorf("adding bookmarks: %w", err)
	}

	return outputFilename, nil
}

// ──────────────────────────────────────────────
// Orchestrator: JSON mode (Mode B)
// ──────────────────────────────────────────────

func processJSONMode(opts JSONModeOpts) (string, error) {
	outputFilename := titleToFilename(opts.Title)
	if opts.Title == "" {
		outputFilename = titleToFilename(folderToTitle(
			strings.TrimSuffix(filepath.Base(opts.PDFPath), filepath.Ext(opts.PDFPath)),
		))
	}

	// Parse and validate JSON
	var raw interface{}
	if err := json.Unmarshal(opts.JSONData, &raw); err != nil {
		return "", &InvalidJsonError{Message: fmt.Sprintf("Ungültiges JSON: %s", err.Error())}
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return "", &InvalidJsonError{Message: "JSON muss eine Liste von Einträgen sein."}
	}

	entries, err := loadJSONEntries(arr, opts.PDFPath)
	if err != nil {
		return "", err
	}

	// Extract and distribute source bookmarks
	sourceOutline, _ := extractOutline(opts.PDFPath)
	distributeOutline(sourceOutline, entries)

	workDir, err := os.MkdirTemp("", "toc-work-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workDir)

	genOpts := GenerateOpts{
		Title:      opts.Title,
		Subtitle:   opts.Subtitle,
		Font:       opts.Font,
		DotLeaders: opts.DotLeaders,
		ShowLines:  opts.ShowLines,
		LineColor:  opts.LineColor,
		BgColor:    opts.BgColor,
		FontColor:  opts.FontColor,
		JSONMode:   true,
	}

	tocPDF, tocPages, err := compileTypst(entries, workDir, genOpts)
	if err != nil {
		return "", err
	}

	// Merge TOC + existing PDF
	mergedPath := filepath.Join(workDir, "merged.pdf")
	if err := mergePDFFiles([]string{tocPDF, opts.PDFPath}, mergedPath); err != nil {
		return "", fmt.Errorf("merging PDFs: %w", err)
	}

	// Add TOC links
	annotatedPath := filepath.Join(workDir, "annotated.pdf")
	if err := addTocLinks(mergedPath, annotatedPath, entries, tocPages,
		opts.Title != "", opts.Subtitle != "", opts.ShowLines); err != nil {
		return "", fmt.Errorf("adding TOC links: %w", err)
	}

	// Add bookmarks
	bookmarkTitle := coalesce(opts.Title, opts.Subtitle, defaultTocLabel(opts.Lang))
	if err := addBookmarks(annotatedPath, opts.OutputPath, entries, bookmarkTitle); err != nil {
		return "", fmt.Errorf("adding bookmarks: %w", err)
	}

	return outputFilename, nil
}
