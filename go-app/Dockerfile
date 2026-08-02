# ── Build stage ──────────────────────────────
FROM golang:1.25 AS builder

WORKDIR /build

COPY go-app/go.mod go-app/go.sum ./
RUN go mod download

COPY go-app/*.go ./
RUN CGO_ENABLED=0 go build -o toc-service .

# ── Runtime stage ────────────────────────────
FROM debian:bookworm-slim

# System dependencies: fontconfig for Typst font discovery
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        fontconfig \
        wget \
        xz-utils \
    && rm -rf /var/lib/apt/lists/*

# Install Typst (static binary from GitHub)
ARG TYPST_VERSION=0.13.1
RUN wget -qO typst.tar.xz \
    "https://github.com/typst/typst/releases/download/v${TYPST_VERSION}/typst-x86_64-unknown-linux-musl.tar.xz" \
    && tar -xf typst.tar.xz \
    && mv typst-x86_64-unknown-linux-musl/typst /usr/local/bin/typst \
    && chmod +x /usr/local/bin/typst \
    && rm -rf typst.tar.xz typst-x86_64-unknown-linux-musl \
    # wget and xz-utils are only needed to fetch Typst; dropping them keeps
    # GPL-3 tooling we do not intentionally ship out of the runtime image.
    && apt-get purge -y --auto-remove wget xz-utils \
    && rm -rf /var/lib/apt/lists/*

# Fonts (IBM Plex Sans, Roboto, Inter)
COPY fonts/ /usr/share/fonts/
RUN fc-cache -fv

WORKDIR /app

# Binary from build stage
COPY --from=builder /build/toc-service .

# Templates and static files
COPY go-app/templates/ ./templates/
COPY static/ ./static/
COPY fonts/ ./fonts/

# Attribution required by the bundled third-party licences
COPY THIRD-PARTY-NOTICES.md LICENSE ./
COPY licenses/ ./licenses/

# Non-root user
RUN useradd --create-home --uid 10001 appuser \
    && mkdir -p /app/data \
    && chown appuser:appuser /app/data
USER appuser

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD ["./toc-service", "-healthcheck"]

CMD ["./toc-service"]
