// toc_template.typ – Inhaltsverzeichnis im Stil des Lebenslaufs
// Wird von build_toc.py automatisch mit Daten befüllt und kompiliert.
//
// DESIGN ANPASSEN:
//   - Balkenfarbe: section-line ändern
//   - Schriftart: font-Parameter unten ändern
//   - Schriftgröße: size-Parameter anpassen
//   - Seitenränder: margin-Werte ändern
//   - Füllzeichen: In der entry-Funktion repeat[.] anpassen
//   - Abstand zwischen Einträgen: v(0.65em) anpassen

// SECTION_LINE_DEF_PLACEHOLDER

#set page(
  paper: "a4",
  margin: (top: 2cm, bottom: 2cm, left: 2.5cm, right: 2.5cm),
  footer: [],
  // BG_COLOR_PLACEHOLDER
)

// FONT_PLACEHOLDER

#v(0.5cm)

// SECTION_LINE_TOP_PLACEHOLDER

// TITLE_SUBTITLE_PLACEHOLDER

#v(0.8cm)

// ENTRY_FUNCTION_PLACEHOLDER

// --- Einträge (automatisch von Python eingefügt) ---
// ENTRIES_PLACEHOLDER
#v(0.8cm)
// SECTION_LINE_BOTTOM_PLACEHOLDER
