# Third-Party Notices

TOC Service is distributed under the MIT License (see [LICENSE](LICENSE)). It
bundles third-party components whose licences require attribution. This file
lists them; the verbatim licence texts live in [`licenses/`](licenses/) and in
`fonts/*/LICENSE.txt`.

**Nothing in this project prohibits redistribution.** Every bundled component
is under a permissive licence (MIT, BSD-2/3-Clause, Apache-2.0, SIL OFL-1.1).
There is no copyleft in the compiled binary or the vendored browser assets.

**Scope.** These obligations apply when you distribute the source tree or the
container image. Operating the service over a network is not distribution
under any of these licences — none of them is AGPL.

This file was generated from the actual build: the Go module list comes from
`go list -deps` against the shipped binary, and each licence text was copied
from the module cache or the upstream package rather than transcribed.

---

## Go modules linked into the binary

Twenty modules are compiled in. Licence texts: [`licenses/go/`](licenses/go/).

| Module | Version | Licence | Copyright |
|---|---|---|---|
| github.com/pdfcpu/pdfcpu | v0.11.1 | Apache-2.0 | The pdfcpu Authors |
| github.com/clipperhouse/uax29/v2 | v2.2.0 | MIT | 2020 Matt Sherman |
| github.com/dustin/go-humanize | v1.0.1 | MIT | 2005-2008 Dustin Sallings |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause | 2009, 2014 Google Inc. |
| github.com/hhrutter/lzw | v1.0.0 | BSD-3-Clause | 2009 The Go Authors |
| github.com/hhrutter/pkcs7 | v0.2.0 | MIT | 2015 Andrew Smith |
| github.com/hhrutter/tiff | v1.0.2 | BSD-3-Clause | 2009 The Go Authors |
| github.com/mattn/go-runewidth | v0.0.19 | MIT | 2016 Yasuhiro Matsumoto |
| github.com/pkg/errors | v0.9.1 | BSD-2-Clause | 2015 Dave Cheney |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748 | BSD-3-Clause | 2012 The Go Authors |
| golang.org/x/crypto | v0.43.0 | BSD-3-Clause | 2009 The Go Authors |
| golang.org/x/exp | v0.0.0-20251023183803 | BSD-3-Clause | 2009 The Go Authors |
| golang.org/x/image | v0.32.0 | BSD-3-Clause | 2009 The Go Authors |
| golang.org/x/sys | v0.37.0 | BSD-3-Clause | 2009 The Go Authors |
| golang.org/x/text | v0.30.0 | BSD-3-Clause | 2009 The Go Authors |
| gopkg.in/yaml.v2 | v2.4.0 | Apache-2.0 | 2011-2016 Canonical Ltd. |
| modernc.org/libc | v1.67.6 | BSD-3-Clause | 2017 The Libc Authors |
| modernc.org/mathutil | v1.7.1 | BSD-3-Clause | 2014 The mathutil Authors |
| modernc.org/memory | v1.11.0 | BSD-3-Clause | 2017 The Memory Authors |
| modernc.org/sqlite | v1.46.1 | BSD-3-Clause | 2017 The Sqlite Authors |

`gopkg.in/yaml.v2` ships a NOTICE file alongside its licence, reproduced at
`licenses/go/gopkg.in_yaml.v2.NOTICE.txt` as Apache-2.0 §4(d) requires.

The Go standard library and runtime are BSD-3-Clause, © The Go Authors.

### Not linked, despite appearing in go.sum

`go.sum` records checksums for modules that are never compiled in — they are
optional drivers of `jmoiron/sqlx`, which this project does not import. Most
notably **github.com/go-sql-driver/mysql (MPL-2.0)** is *not* part of the
binary; neither are `lib/pq` nor `mattn/go-sqlite3`. Verified by inspecting the
module list embedded in the built binary.

## Browser assets (vendored in `static/`)

Licence texts: [`licenses/js/`](licenses/js/).

| File | Project | Licence | SHA-256 (first 16) |
|---|---|---|---|
| `static/pdf-lib.min.js` | pdf-lib | MIT | `0f9a5cad07941f08` |
| `static/fontkit.umd.min.js` | fontkit | MIT | `d8df561b9fba98e2` |
| `static/tailwindcss.js` | Tailwind CSS (Play CDN build) | MIT | `176e894661aa9cdc` |

These are vendored minified builds that record no version string, so they are
identified by content hash instead of a version number.

Two provenance caveats worth knowing:

- **fontkit publishes no licence text.** Neither its npm package nor its
  GitHub repository contains a LICENSE file; MIT is declared only in
  `package.json`. See the note at the top of `licenses/js/fontkit.txt`.
- **`pdf-lib.min.js` embeds tslib**, which carries its own Apache-2.0 notice
  (© Microsoft Corporation) inside the bundle. `tailwindcss.js` likewise
  embeds MIT-licensed helpers (© Jon Schlinkert).

## Fonts (bundled in `fonts/`, served at `/fonts/`)

| Family | Licence | Text |
|---|---|---|
| IBM Plex Sans | SIL OFL-1.1 | `fonts/IBMPlexSans/LICENSE.txt` |
| Inter | SIL OFL-1.1 | `fonts/Inter/LICENSE.txt` |
| Roboto | Apache-2.0 | `fonts/Roboto/LICENSE.txt` |

OFL-1.1 reserves the names "Plex" and "Inter": a modified version must be
renamed, and the fonts may not be sold on their own. Both are bundled
unmodified here.

## Typst

The container downloads the Typst CLI **v0.13.1** (Apache-2.0, © The Typst
Project Developers) from the upstream release page and installs it at
`/usr/local/bin/typst`. Licence text: [`licenses/typst.txt`](licenses/typst.txt).

## Container base image

The runtime image derives from `debian:bookworm-slim`, which carries roughly a
hundred Debian packages under a mix of GPL, LGPL, BSD, MIT and other licences.
These are redistributable; Debian satisfies the GPL's source-availability
requirement through its public archive at <https://sources.debian.org>. Each
package's terms are in the image under `/usr/share/doc/<package>/copyright`.

Build-only tooling (`wget`, `xz-utils`) is purged after Typst is installed, so
it is not shipped in the runtime image.
