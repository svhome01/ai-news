# Personalized AI News Station

ニュースの自動収集、AIによる要約、VOICEVOXによる音声合成、そしてHome Assistantを通じた自宅スピーカー（svhome02）からの再生を統合するシステム。

## 🚀 Quick Start
- **Web UI**: [https://news.futamura.dev](https://news.futamura.dev)
- **Local API**: http://192.168.0.13:8181

## 📂 Project Structure & Docs
プロジェクトの詳細な文脈は以下のドキュメントに集約されています。
- [CLAUDE.md](./CLAUDE.md) - 技術スタック、インフラ詳細、開発ルール（AIとの作業用）
- [PLAN.md](./PLAN.md) - 現在の進捗、タスクリスト、Next Steps
- [docs/](./docs/) - 各種設計書やAPI仕様書

## 🛠 Tech Stack
- **Backend**: Go (domain/repository pattern)
- **Database**: SQLite3
- **Infrastructure**: Docker (svhome01), LXC, SMB Mount
- **Integration**: Home Assistant, VOICEVOX, Cloudflare Tunnel, Navidrome

## 📝 Note
このプロジェクトは Claude Code を使用して開発されています。
新しいセッションを開始する際は、`CLAUDE.md` を読み込ませることで環境設定を同期できます。
