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

# データディレクトリ（SQLite DB が作成されるまで空でよい）
ls -la /opt/stacks/ai-news/data/
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
| [migrations/001_initial.sql](migrations/001_initial.sql) | SQLite スキーマ全量 |
| [entrypoint.sh](entrypoint.sh) | コンテナ起動スクリプト |
| [PLAN.md](PLAN.md) | 全フェーズ実装設計書 |
