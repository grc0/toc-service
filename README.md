# TOC Service – PDF Inhaltsverzeichnis Generator

Web-Service zur automatischen Erzeugung klickbarer Inhaltsverzeichnisse für PDF-Sammlungen. Implementiert in Go (Standard-`net/http`-Mux), nutzt [Typst](https://typst.app) zum Rendern der TOC-Seite und [pdfcpu](https://github.com/pdfcpu/pdfcpu) zum Zusammenführen und Setzen von Lesezeichen/Links.

## Schnellstart mit Docker

```bash
docker compose up --build
```

Dann öffne **http://localhost:8000** im Browser.

Die Usage-Datenbank wird im Volume `usage-data` (`/app/data` im Container) persistiert.

## Funktionen

### Modus A – Mehrere PDFs hochladen
Lade einzelne PDF-Dateien hoch. Der Service erstellt ein Inhaltsverzeichnis, fügt alle PDFs zusammen und liefert eine einzelne PDF-Datei mit klickbaren Links und PDF-Lesezeichen.

### Modus B – Bestehendes PDF + JSON
Lade ein bereits zusammengefügtes PDF und eine JSON-Datei mit den Einträgen hoch. Der Service erstellt ein Inhaltsverzeichnis und stellt es dem PDF voran.

**JSON-Format:**
```json
[
  {"title": "Arbeitszeugnis SwissSign AG", "page": 1},
  {"title": "Zwischenzeugnis DCL Data Care AG", "page": 3}
]
```

### Lokaler Modus (im Browser)
Unter **`/local`** ist eine Variante verfügbar, die vollständig im Browser läuft – keine PDF-Daten verlassen den Rechner. Verwendet [pdf-lib](https://pdf-lib.js.org/) und [fontkit](https://github.com/foliojs/fontkit). Funktional weitgehend identisch zur Server-Variante (beide Modi inkl. `merge_only`).

## Lokale Entwicklung (ohne Docker)

### Voraussetzungen
- Go 1.25+
- [Typst](https://typst.app) (muss im `PATH` sein)
- IBM Plex Sans / Roboto / Inter im System installiert (oder Fonts aus `fonts/` einbinden)

### Bauen und starten
```bash
cd go-app
go build -o toc-service .
./toc-service
```

Standard-Port: **8000**.

### Tests
```bash
cd go-app
go test ./...
```

## Web-UI Routen

| Pfad | Beschreibung |
|------|--------------|
| `/` | Haupt-Frontend (Studio: Live-Vorschau, per-Datei-Umbenennung, Dark/Light) |
| `/local` | Browser-Only-Variante (keine Uploads) |
| `/howto` | Anleitung |
| `/usage` | Aufrufstatistik (UI) |

## API-Endpoints

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| `GET`  | `/health` | Health Check (inkl. installierter Typst-Version) |
| `POST` | `/api/generate/folder` | Modus A: Mehrere PDFs zusammenführen |
| `POST` | `/api/generate/json` | Modus B: PDF + JSON-Einträge |
| `GET`  | `/api/fonts` | Verfügbare Schriftarten |
| `GET`  | `/api/colors` | Vordefinierte Balkenfarben |
| `GET`  | `/api/bg-colors` | Vordefinierte Hintergrundfarben |
| `GET`  | `/api/font-colors` | Vordefinierte Schriftfarben |
| `GET`  | `/api/usage` | Statistik + letzte Aufrufe (`?limit=1..500`, Default 50) |

Jeder Request bekommt einen `X-Request-ID`-Header (übergebener Wert wird übernommen, sonst neu generiert).

### Direkte Nutzung per REST API

**Modus A – Mehrere PDFs zusammenführen:**
```bash
curl -X POST http://localhost:8000/api/generate/folder \
  -F "files=@dokument1.pdf" \
  -F "files=@dokument2.pdf" \
  -F "title=Bewerbungsunterlagen" \
  -F "subtitle=2024" \
  -F "font=Inter" \
  -F "dot_leaders=true" \
  -F "show_lines=true" \
  -F "line_color=#2563EB" \
  -o ausgabe.pdf
```

**Modus B – Bestehendes PDF + JSON:**
```bash
curl -X POST http://localhost:8000/api/generate/json \
  -F "pdf=@dokument.pdf" \
  -F "json_file=@eintraege.json" \
  -F "title=Inhaltsverzeichnis" \
  -o ausgabe.pdf
```

**Optionale Parameter für beide Modi:**

| Parameter | Default | Beschreibung |
|-----------|---------|--------------|
| `title` | – | Titel über dem Inhaltsverzeichnis |
| `subtitle` | – | Untertitel |
| `font` | `IBM Plex Sans` | Schriftart (`IBM Plex Sans`, `Roboto`, `Inter`) |
| `dot_leaders` | `true` | Punktlinie zwischen Titel und Seitenzahl |
| `show_lines` | `true` | Zierbalken ober- und unterhalb der Einträge |
| `line_color` | `#1A2980` | Farbe der Zierbalken (Hex-Wert) |
| `bg_color` | `#FFFFFF` | Seitenhintergrundfarbe |
| `font_color` | `#000000` | Textfarbe |
| `lang` | `de` | Sprache (`de`, `en`) – steuert u. a. den Default-Titel |

**Nur Modus A:**

| Parameter | Default | Beschreibung |
|-----------|---------|--------------|
| `merge_only` | `false` | Nur zusammenführen mit PDF-Lesezeichen, ohne sichtbare Inhaltsverzeichnisseite |
| `titles` | – | Optional, mehrfach. Eintrags-Titel pro PDF, in derselben Reihenfolge wie die `files`. Leerer String = aus dem Dateinamen ableiten. Nützlich z. B. für die Studio-Umbenennen-Funktion. |

## Konfiguration (Environment Variables)

| Variable | Default | Beschreibung |
|----------|---------|--------------|
| `MAX_UPLOAD_MB` | `50` | Max. Upload-Größe für PDF-Dateien |
| `MAX_JSON_MB` | `2` | Max. Upload-Größe für JSON-Dateien |
| `MAX_FILES` | `100` | Max. Anzahl PDF-Dateien pro Anfrage |
| `TYPST_TIMEOUT_SECONDS` | `30` | Timeout für Typst-Kompilierung |
| `TYPST_VERSION_TIMEOUT_SECONDS` | `2` | Timeout für Typst-Version-Check |

## Usage-Logging

Aufrufe werden in einer SQLite-Datenbank unter `data/usage.db` protokolliert (Tabelle `api_calls`, WAL-Modus). Erfasst werden u. a. Endpoint, Modus, Dateianzahl, gewählte Optionen, Status, Dauer und ggf. Fehlermeldung. Keine PDF-Inhalte oder Dateinamen werden gespeichert.

## Deployment

- **Docker / Compose**: siehe `go-app/Dockerfile` und `docker-compose.yml`. Multi-Stage-Build, Non-Root-User, Healthcheck auf `/health`.
- **Fly.io**: `fly.toml` ist vorkonfiguriert (Region `fra`, persistentes Volume `usage_data` auf `/app/data`, Auto-Stop/Start).

## Projektstruktur

```
toc-service/
├── go-app/
│   ├── main.go              # HTTP-Handler, Routen, Middleware
│   ├── toc_engine.go        # PDF-Verarbeitung (pdfcpu + Typst)
│   ├── usage_log.go         # SQLite-Logging
│   ├── templates/
│   │   └── toc_template.typ # Typst-Design-Template
│   ├── *_test.go
│   └── Dockerfile
├── static/
│   ├── index.html           # Studio-Frontend (Live-Vorschau, Dark/Light)
│   ├── local.html           # Browser-Only Variante (pdf-lib + fontkit)
│   ├── howto.html
│   ├── usage.html
│   ├── pdf-lib.min.js, fontkit.umd.min.js, tailwindcss.js
│   └── logos/
├── fonts/                   # IBM Plex Sans, Roboto, Inter
├── data/                    # SQLite (zur Laufzeit)
├── docker-compose.yml
├── fly.toml
└── beispiel_eintraege.json  # Beispiel-JSON für Modus B
```

## Security
Bitte melde Sicherheitslücken privat. Siehe [SECURITY.md](SECURITY.md).

---

# TOC Service – PDF Table of Contents Generator

Web service for generating clickable tables of contents for PDF collections. Implemented in Go (standard-library `net/http`), uses [Typst](https://typst.app) to render the TOC page and [pdfcpu](https://github.com/pdfcpu/pdfcpu) to merge PDFs and add bookmarks/links.

## Quick start with Docker

```bash
docker compose up --build
```

Then open **http://localhost:8000** in your browser.

The usage database is persisted in the `usage-data` volume (`/app/data` inside the container).

## Features

### Mode A – Upload multiple PDFs
Upload individual PDF files. The service generates a table of contents, merges all PDFs and returns a single PDF with clickable links and PDF bookmarks.

### Mode B – Existing PDF + JSON
Upload an already-merged PDF and a JSON file with entries. The service generates a TOC and prepends it to the PDF.

**JSON format:**
```json
[
  {"title": "Employment Reference SwissSign AG", "page": 1},
  {"title": "Interim Reference DCL Data Care AG", "page": 3}
]
```

### Local mode (in-browser)
A fully client-side variant is available at **`/local`** – no PDF data ever leaves your browser. Built with [pdf-lib](https://pdf-lib.js.org/) and [fontkit](https://github.com/foliojs/fontkit). Functionally close to the server variant (both modes incl. `merge_only`).

## Local development (without Docker)

### Requirements
- Go 1.25+
- [Typst](https://typst.app) (must be in `PATH`)
- IBM Plex Sans / Roboto / Inter installed system-wide (or via `fonts/`)

### Build & run
```bash
cd go-app
go build -o toc-service .
./toc-service
```

Default port: **8000**.

### Tests
```bash
cd go-app
go test ./...
```

## Web UI routes

| Path | Description |
|------|-------------|
| `/` | Main frontend (Studio: live preview, per-file rename, dark/light) |
| `/local` | Browser-only variant (no uploads) |
| `/howto` | Usage guide |
| `/usage` | Call statistics (UI) |

## API endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET`  | `/health` | Health check (incl. installed Typst version) |
| `POST` | `/api/generate/folder` | Mode A: merge multiple PDFs |
| `POST` | `/api/generate/json` | Mode B: PDF + JSON entries |
| `GET`  | `/api/fonts` | Available fonts |
| `GET`  | `/api/colors` | Predefined bar colors |
| `GET`  | `/api/bg-colors` | Predefined background colors |
| `GET`  | `/api/font-colors` | Predefined text colors |
| `GET`  | `/api/usage` | Stats + recent calls (`?limit=1..500`, default 50) |

Every request gets an `X-Request-ID` header (passed-in value is preserved, otherwise generated).

### Direct REST usage

**Mode A – merge multiple PDFs:**
```bash
curl -X POST http://localhost:8000/api/generate/folder \
  -F "files=@document1.pdf" \
  -F "files=@document2.pdf" \
  -F "title=Application Documents" \
  -F "subtitle=2024" \
  -F "font=Inter" \
  -F "dot_leaders=true" \
  -F "show_lines=true" \
  -F "line_color=#2563EB" \
  -o output.pdf
```

**Mode B – existing PDF + JSON:**
```bash
curl -X POST http://localhost:8000/api/generate/json \
  -F "pdf=@document.pdf" \
  -F "json_file=@entries.json" \
  -F "title=Table of Contents" \
  -o output.pdf
```

**Optional parameters for both modes:**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `title` | – | Title above the table of contents |
| `subtitle` | – | Subtitle |
| `font` | `IBM Plex Sans` | Font (`IBM Plex Sans`, `Roboto`, `Inter`) |
| `dot_leaders` | `true` | Dot leaders between title and page number |
| `show_lines` | `true` | Decorative bars above and below entries |
| `line_color` | `#1A2980` | Bar color (hex) |
| `bg_color` | `#FFFFFF` | Page background color |
| `font_color` | `#000000` | Text color |
| `lang` | `de` | Language (`de`, `en`) – also drives the default title |

**Mode A only:**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `merge_only` | `false` | Merge with PDF bookmarks but no visible TOC page |
| `titles` | – | Optional, repeated. Entry title per PDF, in the same order as `files`. Empty string = derive from filename. Used e.g. by the Studio rename feature. |

## Configuration (Environment Variables)

| Variable | Default | Description |
|----------|---------|-------------|
| `MAX_UPLOAD_MB` | `50` | Max upload size for PDF files |
| `MAX_JSON_MB` | `2` | Max upload size for JSON files |
| `MAX_FILES` | `100` | Max number of PDF files per request |
| `TYPST_TIMEOUT_SECONDS` | `30` | Typst compilation timeout |
| `TYPST_VERSION_TIMEOUT_SECONDS` | `2` | Typst version check timeout |

## Usage logging

Calls are logged to a SQLite database at `data/usage.db` (table `api_calls`, WAL mode). Recorded fields include endpoint, mode, file count, chosen options, status, duration and any error message. No PDF contents or filenames are stored.

## Deployment

- **Docker / Compose**: see `go-app/Dockerfile` and `docker-compose.yml`. Multi-stage build, non-root user, healthcheck on `/health`.
- **Fly.io**: `fly.toml` is preconfigured (region `fra`, persistent `usage_data` volume mounted at `/app/data`, auto stop/start).

## Project structure

```
toc-service/
├── go-app/
│   ├── main.go              # HTTP handlers, routes, middleware
│   ├── toc_engine.go        # PDF processing (pdfcpu + Typst)
│   ├── usage_log.go         # SQLite logging
│   ├── templates/
│   │   └── toc_template.typ # Typst design template
│   ├── *_test.go
│   └── Dockerfile
├── static/
│   ├── index.html           # Studio frontend (live preview, dark/light)
│   ├── local.html           # Browser-only variant (pdf-lib + fontkit)
│   ├── howto.html
│   ├── usage.html
│   ├── pdf-lib.min.js, fontkit.umd.min.js, tailwindcss.js
│   └── logos/
├── fonts/                   # IBM Plex Sans, Roboto, Inter
├── data/                    # SQLite (runtime)
├── docker-compose.yml
├── fly.toml
└── beispiel_eintraege.json  # Sample JSON for Mode B
```

## Security
Please report vulnerabilities privately. See [SECURITY.md](SECURITY.md).
