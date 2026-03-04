# SETUP — ai-news セットアップ手順

## 1. 環境概要

```
[svhome01 192.168.0.10]
 └── LXC 103 Docker (192.168.0.13)   ← セットアップ対象
      ├── ai-news         :8181
      ├── VOICEVOX        :50021
      └── playwright-chrome ws:3000 (内部)
```

| SSH エイリアス | 用途 |
|---|---|
| `ssh svhome01-docker` | LXC 103 直接操作（docker compose 実行場所） |
| `ssh svhome01` | Proxmox ホスト操作 |

**作業ディレクトリ:** `/opt/stacks/ai-news/`（HOST SSD bind mount 経由）

---

## 2. 初回セットアップ

### 2-1. リポジトリの準備

```bash
ssh svhome01-docker
mkdir -p /opt/stacks/ai-news/data
git clone https://github.com/svhome01/ai-news.git /opt/stacks/ai-news
```

### 2-2. AppArmor 回避ラッパーのインストール ⚠️ LXC 103 必須

> **背景:** Proxmox LXC 103 は AppArmor namespace の policy admin 権限を持たないため、
> `docker build` も `docker compose up` も AppArmor プロファイルの適用に失敗する。
> 以下の Python ラッパーを手動でインストールすることで回避する。
> **サーバー再起動後も再確認が必要。**

#### ① apparmor_parser ラッパー（Docker daemon 事前チェック対策）

```bash
cp /usr/sbin/apparmor_parser /usr/sbin/apparmor_parser.real
cat > /tmp/aa_parser.py << 'EOF'
#!/usr/bin/env python3
# Docker が docker-default プロファイルをロードしようとする事前チェックを無害化
import sys
sys.exit(0)
EOF
python3 -c "
import os, shutil
os.chmod('/tmp/aa_parser.py', 0o755)
shutil.move('/tmp/aa_parser.py', '/usr/sbin/apparmor_parser')
"
```

#### ② runc ラッパー（BuildKit の直接 runc 呼び出し対策）

```bash
cp /usr/bin/runc /usr/bin/runc.real
python3 - << 'PYEOF'
script = r'''#!/usr/bin/env python3
import json, os, sys

REAL_RUNC = "/usr/bin/runc.real"
GLOBAL_VALUE_OPTS = {"--log", "-l", "--log-format", "--root", "--criu", "--rootless"}

def find_subcommand(args):
    i = 0
    while i < len(args):
        a = args[i]
        if a.startswith("-"):
            if "=" in a: i += 1
            elif a in GLOBAL_VALUE_OPTS: i += 2
            else: i += 1
        else:
            return a
    return None

def find_bundle(args):
    for i, arg in enumerate(args):
        if arg in ("--bundle", "-b") and i + 1 < len(args):
            return args[i + 1]
        if arg.startswith("--bundle="):
            return arg.split("=", 1)[1]
    return os.getcwd()

def main():
    args = sys.argv[1:]
    if find_subcommand(args) in ("create", "run"):
        config = os.path.join(find_bundle(args), "config.json")
        if os.path.exists(config):
            try:
                with open(config) as f: spec = json.load(f)
                if spec.get("process", {}).get("apparmorProfile"):
                    del spec["process"]["apparmorProfile"]
                    with open(config, "w") as f: json.dump(spec, f)
            except Exception: pass
    os.execv(REAL_RUNC, [REAL_RUNC] + args)

if __name__ == "__main__": main()
'''
with open('/tmp/runc_wrap.py', 'w') as f: f.write(script)
import os, shutil
os.chmod('/tmp/runc_wrap.py', 0o755)
shutil.move('/tmp/runc_wrap.py', '/usr/bin/runc')
print('runc wrapper installed')
PYEOF
```

#### ③ containerd-shim-runc-v2 ラッパー（通常コンテナ起動対策）

```bash
cp /usr/bin/containerd-shim-runc-v2 /usr/bin/containerd-shim-runc-v2.real
python3 - << 'PYEOF'
script = r'''#!/usr/bin/env python3
import json, os, sys

REAL_SHIM   = "/usr/bin/containerd-shim-runc-v2.real"
BUNDLE_BASE = "/run/containerd/io.containerd.runtime.v2.task"

def main():
    args = sys.argv[1:]
    namespace, container_id = "moby", None
    i = 0
    while i < len(args):
        if args[i] == "-namespace" and i + 1 < len(args):
            namespace = args[i + 1]; i += 2
        elif args[i] == "-id" and i + 1 < len(args):
            container_id = args[i + 1]; i += 2
        else: i += 1
    if container_id:
        config = f"{BUNDLE_BASE}/{namespace}/{container_id}/config.json"
        try:
            with open(config) as f: spec = json.load(f)
            if spec.get("process", {}).get("apparmorProfile"):
                del spec["process"]["apparmorProfile"]
                with open(config, "w") as f: json.dump(spec, f)
        except Exception: pass
    os.execv(REAL_SHIM, [REAL_SHIM] + args)

if __name__ == "__main__": main()
'''
with open('/tmp/shim_wrap.py', 'w') as f: f.write(script)
import os, shutil
os.chmod('/tmp/shim_wrap.py', 0o755)
shutil.move('/tmp/shim_wrap.py', '/usr/bin/containerd-shim-runc-v2')
print('containerd-shim wrapper installed')
PYEOF
```

#### ④ daemon.json の設定

```bash
cat > /etc/docker/daemon.json << 'EOF'
{
  "runtimes": {
    "runc-no-apparmor": {
      "path": "/usr/local/bin/runc-no-apparmor"
    }
  },
  "default-runtime": "runc-no-apparmor"
}
EOF
cp /usr/bin/runc /usr/local/bin/runc-no-apparmor
systemctl restart docker
systemctl is-active docker   # → active であることを確認
```

#### ⑤ ラッパー確認コマンド

```bash
# いずれも #!/usr/bin/env python3 が出力されれば OK
head -1 /usr/sbin/apparmor_parser
head -1 /usr/bin/runc
head -1 /usr/bin/containerd-shim-runc-v2
```

### 2-3. .env の作成

```bash
cd /opt/stacks/ai-news
cp .env.example .env
vim .env   # 以下の値を実際の認証情報に書き換える
```

必須入力項目:

| 変数 | 説明 |
|---|---|
| `GEMINI_API_KEY` | Google AI Studio で取得した API キー |
| `NAVIDROME_USER` / `NAVIDROME_PASS` | Navidrome ログイン情報 |
| `SMB_USER` / `SMB_PASS` | Samba（192.168.0.22）認証情報 |
| `GCLOUD_TTS_KEY` | Google Cloud TTS API キー（省略可: VOICEVOX 専用カテゴリのみの場合） |
| `GCLOUD_TTS_VOICE` | Google Cloud TTS デフォルト音声（省略時: `ja-JP-Neural2-B`） |

> **注意:** `.env` に値を書くだけでなく、`docker-compose.yml` の `environment:` セクションにも明示が必要。追加済み変数の確認は `docker compose exec ai-news env | grep GCLOUD` で確認可能。

### 2-4. ビルド・起動

```bash
cd /opt/stacks/ai-news
docker compose build
docker compose up -d
```

---

## 3. 動作確認

```bash
cd /opt/stacks/ai-news

# コンテナ状態（3つすべて Up であることを確認）
docker compose ps

# ai-news ヘルスチェック
curl http://localhost:8181/healthz
# → ok

# VOICEVOX バージョン確認
curl http://localhost:50021/version
# → "0.21.0"

# Web UI 確認（Phase 2 デプロイ後）
curl -s -o /dev/null -w '%{http_code}' http://localhost:8181/
# → 200

# VOICEVOX スピーカー一覧 API（Phase 2 デプロイ後）
curl -s http://localhost:8181/api/voicevox/speakers | head -c 200

# データディレクトリ（SQLite DB + WAL ファイルが存在することを確認）
ls -la /opt/stacks/ai-news/data/
# → news.db, news.db-wal, news.db-shm が存在すれば正常
```

---

## 4. サーバー再起動後の確認

> ⚠️ Python ラッパーはファイルシステムに永続化されているが、Docker や runc が
> パッケージ更新で上書きされる場合がある。再起動後は必ず確認すること。

```bash
# 1. ラッパーが生きているか確認
head -1 /usr/sbin/apparmor_parser        # → #!/usr/bin/env python3
head -1 /usr/bin/runc                    # → #!/usr/bin/env python3
head -1 /usr/bin/containerd-shim-runc-v2 # → #!/usr/bin/env python3

# 2. Docker が起動しているか確認
systemctl is-active docker               # → active

# 3. コンテナが自動起動しているか確認（restart: unless-stopped）
docker compose -f /opt/stacks/ai-news/docker-compose.yml ps
```

ラッパーが壊れていた場合は「2-2. AppArmor 回避ラッパーのインストール」を再実行する。

---

## 5. よくある問題と対処

### ビルドが `unable to apply apparmor profile` で失敗する

```
runc run failed: unable to start container process: error during container init:
unable to apply apparmor profile: apparmor failed to apply profile:
write fsmount:fscontext:proc/thread-self/attr/apparmor/exec: no such file or directory
```

**原因:** runc ラッパーが機能していない。
**対処:** `head -1 /usr/bin/runc` を確認し、バイナリが上書きされていれば 2-2② を再実行。

### `docker compose up` が `AppArmor enabled on system but docker-default profile could not be loaded` で失敗する

**原因:** apparmor_parser ラッパーが機能していない、または Docker daemon が再起動されていない。
**対処:** `head -1 /usr/sbin/apparmor_parser` を確認し、2-2① を再実行後 `systemctl restart docker`。

### Docker daemon が起動しない（`/etc/docker/daemon.json` が原因の可能性）

```bash
# daemon.json を一時退避して Docker を起動
mv /etc/docker/daemon.json /tmp/daemon.json.bak
systemctl start docker
# 起動確認後に daemon.json を復元
mv /tmp/daemon.json.bak /etc/docker/daemon.json
systemctl restart docker
```

### voicevox コンテナが起動直後に unhealthy になる

LXC 103 では `docker exec` も AppArmor 制限を受けるため、healthcheck は無効化済み（`healthcheck: disable: true`）。voicevox が `Up` 状態であれば正常。
VOICEVOX の起動完了は `curl http://localhost:50021/version` で確認する。

---

## 6. 関連ファイル

| ファイル | 用途 |
|---|---|
| [docker-compose.yml](docker-compose.yml) | 3サービス定義 |
| [Dockerfile](Dockerfile) | ai-news マルチステージビルド |
| [.env.example](.env.example) | 環境変数テンプレート |
| [migrations/001_initial.sql](migrations/001_initial.sql) | SQLite 初期スキーマ全量 |
| [migrations/002_gcloud_tts.sql](migrations/002_gcloud_tts.sql) | tts_engine CHECK に 'gcloud' 追加 (migration v2) |
| [migrations/003_per_category_speed.sql](migrations/003_per_category_speed.sql) | category_settings.speed_scale 追加 (migration v3) |
| [entrypoint.sh](entrypoint.sh) | コンテナ起動スクリプト |
| [cmd/server/main.go](cmd/server/main.go) | エントリーポイント・DI配線・ルーティング |
| [internal/](internal/) | ドメイン・リポジトリ・ユースケース・インフラ・ハンドラー |
| [templates/](templates/) | html/template + HTMX テンプレート群 |
| [PLAN.md](PLAN.md) | 全フェーズ実装設計書 |

---

## 7. Phase 2 デプロイ（コード更新手順）

サーバー上での標準的な更新フロー:

```bash
ssh svhome01-docker
cd /opt/stacks/ai-news

# コード取得
git pull

# Docker イメージを再ビルドして再起動
docker compose build ai-news
docker compose up -d ai-news

# ログで起動確認
docker compose logs -f --tail=30 ai-news
# → "ai-news listening on :8181" が出れば正常
```

> **注意:** `go mod tidy` はローカルでは不要（Dockerfile 内の `RUN go mod download` が処理する）。
> 依存ライブラリを追加した場合はローカルで `go mod tidy` → `go.sum` をコミットしてからプッシュ。

---

## 8. Cloudflare Tunnel 設定

**LXC 101 (`cloudflared` コンテナが動作するサーバー) での作業。**

### 8-1. ingress ルールの追加

```bash
ssh svhome01 "pct exec 101 -- bash -c \
  \"sed -i '/- service: http_status:404/i\\\\  - hostname: news.futamura.dev\\n    service: http://192.168.0.13:8181' \
  /etc/cloudflared/config.yml\""
ssh svhome01 "pct exec 101 -- systemctl restart cloudflared"
```

> 手動編集の場合は `pct enter 101` 後に `/etc/cloudflared/config.yml` を vim で編集する。
> `- service: http_status:404`（catch-all）より**前**に ai-news エントリを挿入すること。

```yaml
# 追加するエントリ（catch-all の直前に配置）
ingress:
  # ... 既存エントリ ...
  - hostname: news.futamura.dev
    service: http://192.168.0.13:8181
  - service: http_status:404   # ← catch-all（最後）
```

### 8-2. DNS CNAME レコードの追加

```bash
# LXC 101 内で実行
ssh svhome01 "pct exec 101 -- cloudflared tunnel route dns svhome-tunnel news.futamura.dev"
# → Added CNAME news.futamura.dev ... が出力されれば成功
```

### 8-3. 確認

```bash
curl -sf https://news.futamura.dev/healthz
# → ok
```

---

## 9. Home Assistant スクリプト設定

**前提:** `media_player.svhome02_audio` エンティティが HA に存在すること（後述）。

### 9-1. scripts.yaml への追記

`/config/scripts.yaml`（HA 設定ディレクトリ）に以下を追記:

```yaml
play_tech_news:
  alias: AI News - テックニュース再生
  sequence:
  - action: media_player.play_media
    target:
      entity_id: media_player.svhome02_audio
    data:
      media_content_id: http://192.168.0.13:8181/media/tech/latest
      media_content_type: music
  mode: single
  icon: mdi:radio

play_business_news:
  alias: AI News - ビジネスニュース再生
  sequence:
  - action: media_player.play_media
    target:
      entity_id: media_player.svhome02_audio
    data:
      media_content_id: http://192.168.0.13:8181/media/business/latest
      media_content_type: music
  mode: single
  icon: mdi:radio

stop_news_playback:
  alias: AI News - 再生停止
  sequence:
  - action: media_player.media_stop
    target:
      entity_id: media_player.svhome02_audio
  mode: single
  icon: mdi:stop
```

### 9-2. スクリプトのリロード

```bash
# HAOS SSH 経由（port 22 が開いている場合）
ssh -p 22 root@192.168.0.21 \
  'curl -s -X POST -H "Authorization: Bearer $SUPERVISOR_TOKEN" \
   http://supervisor/core/api/services/script/reload'
# → [] が返れば成功
```

または HA Web UI: **Settings → System → YAML Configuration → Reload Scripts**

### 9-3. 登録確認

```bash
ssh -p 22 root@192.168.0.21 \
  'curl -s -H "Authorization: Bearer $SUPERVISOR_TOKEN" \
   http://supervisor/core/api/states' \
  | python3 -c "import sys,json; [print(s['entity_id']) for s in json.load(sys.stdin) if s['entity_id'].startswith('script.play') or s['entity_id'].startswith('script.stop_news')]"
# → script.play_tech_news
# → script.play_business_news
# → script.stop_news_playback
```

### 9-4. media_player.svhome02_audio の作成（未設定の場合）

`media_player.svhome02_audio` エンティティは別途設定が必要。選択肢:

| 方法 | 概要 |
|---|---|
| USB オーディオパススルー | svhome02 に USB オーディオデバイス接続 → Proxmox UI で VM 201 に USB デバイス追加 → HA が自動検出 |
| MPD Integration | svhome02 上で MPD を起動 → HA [Music Player Daemon integration](https://www.home-assistant.io/integrations/mpd/) を追加 |
| 既存メディアプレイヤー | scripts.yaml の `entity_id` を `media_player.work_room_nest_mini` など既存エンティティに変更 |

---

## 10. Web UI 使い方

| URL | 機能 |
|---|---|
| `http://192.168.0.13:8181/` | ダッシュボード（カテゴリ別記事一覧・最新放送情報） |
| `http://192.168.0.13:8181/ui/sources` | ニュースソース管理（追加・編集・削除） |
| `http://192.168.0.13:8181/ui/settings` | アプリ設定・カテゴリ設定 |
| `http://192.168.0.13:8181/ui/system` | 手動実行（スクレイプ・音声生成）・実行ログ |
| `https://news.futamura.dev/` | Cloudflare Tunnel 経由の外部アクセス |

### 主要 API エンドポイント

| メソッド | パス | 用途 |
|---|---|---|
| GET | `/healthz` | ヘルスチェック |
| GET | `/media/{category}/latest` | 最新 MP3 ストリーミング（HA 再生用） |
| POST | `/api/play/{category}` | カテゴリ再生（メタデータ取得） |
| POST | `/api/play/stop` | 再生停止 |
| GET | `/api/broadcasts` | 放送一覧 JSON |
| POST | `/api/pipeline/run` | パイプライン手動実行 |
| GET | `/api/pipeline/{id}` | パイプライン実行状態 |
| GET | `/api/voicevox/speakers` | VOICEVOX スピーカー一覧 |
