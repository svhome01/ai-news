# CLAUDE.md

このファイルはClaude Code (claude.ai/code) がこのリポジトリで作業する際のガイダンスを提供する。

---

## Project Overview

パーソナライズドAIニュースステーション。自宅サーバー (svhome01 Docker LXC 103, 192.168.0.13) 上で動くGoバックエンドとして、ニュース収集・AI要約・音声合成・メディア管理・Web UI提供を担う。

- **公開URL:** `https://news.futamura.dev` (Cloudflare Tunnel経由)
- **LAN URL:** `http://192.168.0.13:8181`
- **音声出力:** svhome02のオーディオ端子 (Home Assistant経由で物理再生)

---

## Infrastructure Architecture

```
[svhome01 192.168.0.10]                    [svhome02 192.168.0.20]
 └── LXC 103 Docker (192.168.0.13)          ├── VM 201 HAOS (192.168.0.21)
      ├── ai-news         :8181 ★            │    └── media_player.svhome02_audio
      ├── VOICEVOX        :50021             ├── LXC 202 Samba (192.168.0.22)
      ├── playwright-chrome ws:3000 (内部)   │    └── //Music (MP3保存先)
      └── Scrutiny Hub    :8080 (既存)       └── LXC 203 Navidrome (192.168.0.23:4533)
```

### ポート使用状況 (LXC 103)

| ポート | サービス | 備考 |
|---|---|---|
| 7900 | Selenium NoVNC | 既存 |
| **8080** | **Scrutiny Hub** | **使用中 — 競合禁止** |
| 8086 | InfluxDB | 既存 |
| 9000 | Portainer | 既存 |
| 50021 | VOICEVOX | ai-news用 |
| **8181** | **ai-news** | ★このアプリ |

---

## SSH Access & Operation Rules

**全てのサーバー作業はSSH経由で実施。ローカルマシンで直接コマンド実行禁止。**

```bash
ssh svhome01-docker   # docker-lxc (LXC 103) 直接操作 — ai-news作業はここ
ssh svhome01          # svhome01 Proxmoxホスト操作
ssh svhome02          # svhome02 Proxmoxホスト操作
# pct経由: ssh svhome01 → pct enter 103
```

### ストレージ構成

```
svhome01 HOST SSD /mnt/app_data/docker/
  └── ai-news/            ← 物理配置 (HOST SSD上)
        ├── data/news.db  ← SQLite DB
        └── ...

docker-lxc /opt/stacks/   ← bind mount (HOST SSD → LXC 103)
  └── ai-news/            ← git clone・docker compose実行場所
```

- `ssh svhome01-docker` 接続後、`/opt/stacks/ai-news/` で作業
- Sambaへの音楽ファイル読み書きはGo側でgo-smb2を使用（cifs-utils/fstab不要）

---

## Tech Stack

| 分類 | 技術 |
|---|---|
| 言語 | Go 1.22 |
| HTTPサーバー | `net/http` 標準 (ServeMux, Go 1.22メソッド+プレフィックスパターン) |
| DB | SQLite — `modernc.org/sqlite` (CGO不要) |
| スケジューラ | `github.com/robfig/cron/v3` |
| テンプレート | `html/template` + HTMX + Pico.css (CDN) |
| SMBクライアント | `github.com/hirochachacha/go-smb2` (CGO不要) |
| RSSパース | `github.com/mmcdole/gofeed` |
| HTMLパース | `github.com/PuerkitoBio/goquery` |
| JSレンダリング | `github.com/playwright-community/playwright-go` (ConnectOverCDP) |
| AI | `google.golang.org/genai` (Gemini API) |
| TTS | VOICEVOX REST API / edge-tts (将来) |
| MP3処理 | ffmpeg (WAV→MP3変換, concat) + ffprobe (duration取得) |
| ID3タグ | `github.com/bogem/id3v2/v2` |
| 環境変数 | `github.com/joho/godotenv` |

---

## Directory Structure

```
ai-news/
├── cmd/server/main.go              # エントリーポイント: DI配線・HTTPサーバー起動
├── internal/
│   ├── domain/                     # ドメインモデル（外部依存なし）
│   │   ├── article.go
│   │   ├── source.go
│   │   ├── broadcast.go
│   │   ├── pipeline.go
│   │   └── errors.go
│   ├── repository/                 # SQLiteアクセス層
│   │   ├── db.go                   # DB open/close, WAL, スキーママイグレーション
│   │   ├── article_repo.go
│   │   ├── source_repo.go
│   │   ├── broadcast_repo.go
│   │   ├── pipeline_repo.go
│   │   ├── settings_repo.go
│   │   ├── category_repo.go
│   │   └── schedule_repo.go
│   ├── usecase/                    # ビジネスロジック
│   │   ├── pipeline_usecase.go     # ScrapeUsecase + GenerateUsecase
│   │   ├── source_usecase.go
│   │   ├── article_usecase.go
│   │   ├── playback_usecase.go
│   │   ├── settings_usecase.go
│   │   ├── category_usecase.go
│   │   └── schedule_usecase.go
│   ├── infra/                      # 外部サービス連携
│   │   ├── scraper/                # RSS / HTTP / Playwright
│   │   ├── gemini/                 # Gemini API
│   │   ├── voicevox/               # VOICEVOX REST
│   │   ├── audio/                  # ffmpeg変換
│   │   ├── storage/                # go-smb2 SMBクライアント
│   │   └── navidrome/              # Subsonic API
│   ├── handler/
│   │   ├── middleware.go
│   │   ├── web/                    # HTMLハンドラー (HTMX)
│   │   └── api/                    # REST APIハンドラー (JSON)
│   ├── scheduler/scheduler.go      # robfig/cron v3
│   └── config/config.go            # 環境変数読み込み
├── migrations/001_initial.sql      # DDL全量 (//go:embedでバイナリ埋め込み)
├── templates/                      # html/templateファイル
├── Dockerfile                      # マルチステージビルド (Go + debian:bookworm-slim)
├── entrypoint.sh                   # 起動時 /data chown → アプリ起動
├── docker-compose.yml              # ai-news + voicevox + playwright-chrome
├── .env.example
└── go.mod
```

**Goレイヤー依存ルール:** `handler → usecase → repository / infra`（逆方向参照禁止）

---

## Key Integration Points

### 音声生成フロー

```
GenerateUsecase
  → Gemini API (記事選定・要約)
  → VOICEVOX (audio_query → synthesis → WAV)
  → ffmpeg (WAV → MP3 + ID3タグ)
  → go-smb2 → //192.168.0.22/Music/ai-news/{category}/ にMP3保存
  → broadcastsテーブルに記録
  → Navidrome Subsonic API startScan
```

### 音声再生フロー

```
HA / ATOM ECHO / Lovelace
  → media_player.play_media (HAOS script)
      media_content_id: http://192.168.0.13:8181/media/{category}/latest
  → GET /media/{category}/latest
  → go-smb2でSMBからMP3読み出し → io.Copy → HTTP音声ストリーム
  → HA → svhome02オーディオ端子

# オプション: ブロードキャストのメタデータ取得
HA → POST /api/play/{category}
  → broadcastRepo.GetLatest
  → 200 JSON {"status":"ok","title":"...","media_url":"...","duration_sec":300}
```

### HA設定例

```yaml
# HA script 設定例（新設計）
# ai-news の /media/{category}/latest を直接 media_content_id に指定して再生
script:
  play_tech_news:
    sequence:
      - action: media_player.play_media
        target:
          entity_id: media_player.svhome02_audio
        data:
          media_content_id: "http://192.168.0.13:8181/media/tech/latest"
          media_content_type: music

  stop_news_playback:
    sequence:
      - action: media_player.media_stop
        target:
          entity_id: media_player.svhome02_audio
```

### Cloudflare Tunnel設定（LXC 101）

```yaml
# /etc/cloudflared/config.yml に追記
ingress:
  - hostname: news.futamura.dev
    service: http://192.168.0.13:8181
```

### Web UI audio URL生成

```go
// Cloudflare Tunnel対応: リクエストHostから動的生成
func mediaURL(r *http.Request, path string) string {
    proto := r.Header.Get("X-Forwarded-Proto")
    host  := r.Header.Get("X-Forwarded-Host")
    if proto == "" { proto = "http" }
    if host  == "" { host  = r.Host }
    return proto + "://" + host + path
}
```

---

## Environment Variables (.env)

```
GEMINI_API_KEY=...
VOICEVOX_URL=http://voicevox:50021
PLAYWRIGHT_CDP_ENDPOINT=ws://playwright-chrome:3000
APP_BASE_URL=http://192.168.0.13:8181
NAVIDROME_URL=http://192.168.0.23:4533
NAVIDROME_USER=...
NAVIDROME_PASS=...
SMB_HOST=192.168.0.22
SMB_USER=...
SMB_PASS=...
SMB_SHARE=Music
SMB_MUSIC_PATH=ai-news
DB_PATH=/data/news.db
PORT=8181
MAX_GEMINI_CONCURRENCY=2
TZ=Asia/Tokyo
```

---

## Writing Conventions

- 解説文は簡潔かつ正確な**体言止め**で記述
- コードブロック内のURLはプレーンテキスト（リンク化禁止）
- コマンドは**冪等性（idempotency）**を保持する設計
- `updated_at` カラムはSQLiteにON UPDATEトリガーがないため、`Update()`メソッド内で `strftime('%Y-%m-%dT%H:%M:%SZ','now')` を明示セット
- 予約語カテゴリ `"stop"` / `"digest"` は `category_usecase.go` の `CreateCategory()` でバリデーション拒否（400エラー）
