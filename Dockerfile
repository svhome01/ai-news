# ── Stage 1: Builder ──────────────────────────────────────────────────────────
# go.mod が go 1.23.0 を要求するため golang:1.23-bookworm を使用
FROM golang:1.23-bookworm AS builder

WORKDIR /app

# 依存解決レイヤーをソースと分離してキャッシュ効率を高める
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0: pure Go ビルド（modernc.org/sqlite, go-smb2 ともにCGO不要）
# GOOS=linux: macOSホストからのクロスコンパイルを保証
RUN CGO_ENABLED=0 GOOS=linux go build -o ai-news ./cmd/server

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM debian:bookworm-slim AS runtime

# ffmpeg : WAV→MP3変換 + digest連結 + ffprobeによるduration取得
# wget   : VOICEVOXヘルスチェック用
# ca-certificates: Gemini API等のHTTPS通信に必要
# ※ PlaywrightのChromium依存は playwright-chrome コンテナに分離済み → 追加不要
# ※ migrations は //go:embed でバイナリ同梱済み → COPY 不要
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    wget \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/ai-news ./
COPY --from=builder /app/entrypoint.sh ./
COPY --from=builder /app/templates ./templates

RUN chmod +x entrypoint.sh

EXPOSE 8181

ENTRYPOINT ["./entrypoint.sh"]
CMD ["./ai-news"]
