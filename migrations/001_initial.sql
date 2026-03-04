-- migrations/001_initial.sql
-- テーブル定義順序: 依存関係に従い pipeline_runs → sources → broadcasts → articles の順に定義
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- ── app_settings: アプリ設定（1行のみ、依存なし、最初に定義）──────
-- CHECK (id = 1) で複数行挿入を禁止。INSERTはOR IGNOREで冪等性を保証
CREATE TABLE IF NOT EXISTS app_settings (
    id                   INTEGER PRIMARY KEY CHECK (id = 1),
    voicevox_speed_scale REAL    NOT NULL DEFAULT 1.0,              -- 話速 (0.5〜2.0)
    gemini_model         TEXT    NOT NULL DEFAULT 'gemini-2.0-flash', -- Gemini APIモデル名
    retention_days       INTEGER NOT NULL DEFAULT 7,                -- 記事・MP3・DBの保持日数
    updated_at           TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
    -- スケジュール設定は schedules テーブルで管理（追加・削除が自由な独立テーブル）
    -- ※ updated_at はSQLiteのON UPDATE非対応のため、settings_repo.goのUpdate()で明示セット必須
);
INSERT OR IGNORE INTO app_settings (id) VALUES (1);                 -- 初期レコードを冪等挿入

-- ── schedules: クロール・音声生成の実行時刻（追加・削除が自由）──────────
-- scrape: 記事クロール専用ジョブの実行時刻
-- generate: 音声生成専用ジョブの実行時刻（Gemini選定→TTS→MP3保存）
-- 各エントリがそのまま独立したcronジョブ (0 {minute} {hour} * * *) として登録される
CREATE TABLE IF NOT EXISTS schedules (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    type    TEXT    NOT NULL CHECK (type IN ('scrape', 'generate')),
    hour    INTEGER NOT NULL CHECK (hour >= 0 AND hour <= 23),
    minute  INTEGER NOT NULL DEFAULT 0 CHECK (minute >= 0 AND minute <= 59),
    enabled INTEGER NOT NULL DEFAULT 1,           -- 0=一時停止, 1=有効
    UNIQUE (type, hour, minute)                   -- 同一種別・同一時刻の重複防止
);
CREATE INDEX IF NOT EXISTS idx_schedules_type_enabled ON schedules (type, enabled);
-- クロール初期設定: 1時間ごと (毎時)
INSERT OR IGNORE INTO schedules (type, hour) VALUES
    ('scrape',  0), ('scrape',  1), ('scrape',  2), ('scrape',  3),
    ('scrape',  4), ('scrape',  5), ('scrape',  6), ('scrape',  7),
    ('scrape',  8), ('scrape',  9), ('scrape', 10), ('scrape', 11),
    ('scrape', 12), ('scrape', 13), ('scrape', 14), ('scrape', 15),
    ('scrape', 16), ('scrape', 17), ('scrape', 18), ('scrape', 19),
    ('scrape', 20), ('scrape', 21), ('scrape', 22), ('scrape', 23);
-- 音声生成初期設定: 朝・昼・夕
INSERT OR IGNORE INTO schedules (type, hour) VALUES
    ('generate',  6), ('generate',  7), ('generate',  8),
    ('generate', 11), ('generate', 12), ('generate', 13),
    ('generate', 18), ('generate', 19), ('generate', 20);

-- ── category_settings: カテゴリごとの設定（拡張可能）────────────────
-- 新カテゴリ追加はここにINSERT + sourcesにURLを登録するだけでパイプラインに自動組み込み
-- sort_orderはUI表示順・パイプライン処理順・digest連結順に使用
-- ── 予約語 (category_usecase.goのCreateCategory()でバリデーション → 400エラー) ──
--   "stop"  : POST /api/play/stop とルーティング競合（Go 1.22 ServeMux はexact matchが優先）
--   "digest": GenerateJob Stage 6がcategory='digest'でbroadcastを自動生成するため
--             ユーザー定義カテゴリと混在するとGetLatest/クリーンアップが誤動作する
CREATE TABLE IF NOT EXISTS category_settings (
    category                  TEXT    PRIMARY KEY,               -- 'tech', 'business', 新カテゴリ等
    display_name              TEXT    NOT NULL,                  -- UI表示名: 'テックニュース' 等
    articles_per_episode      INTEGER NOT NULL DEFAULT 10,       -- 1エピソードの記事数
    summary_chars_per_article INTEGER NOT NULL DEFAULT 200,      -- 1記事あたりの要約文字数
    language                  TEXT    NOT NULL DEFAULT 'ja'
                                      CHECK (language IN ('ja', 'en')),    -- 要約・音声の言語
    tts_engine                TEXT    NOT NULL DEFAULT 'voicevox'
                                      CHECK (tts_engine IN ('voicevox', 'edge-tts', 'gcloud')), -- TTSエンジン選択
    voicevox_speaker_id       INTEGER NOT NULL DEFAULT 3,        -- VOICEVOXのstyle_id (tts_engine='voicevox'時)
    tts_voice                 TEXT,                              -- edge-tts voice名 e.g. 'en-US-GuyNeural' (tts_engine='edge-tts'時)
    enabled                   INTEGER NOT NULL DEFAULT 1,        -- 0=スキップ, 1=パイプライン実行対象
    sort_order                INTEGER NOT NULL DEFAULT 0,        -- UI表示順・digest連結順
    created_at                TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_category_settings_enabled    ON category_settings (enabled);
CREATE INDEX IF NOT EXISTS idx_category_settings_sort_order ON category_settings (sort_order);
-- 初期カテゴリ: tech (ずんだもん ノーマル, style_id=3) / business (四国めたん ノーマル, style_id=2)
-- 英語カテゴリは後で追加: INSERT INTO category_settings (...) VALUES ('tech_en', ..., 'en', 'edge-tts', 3, 'en-US-GuyNeural', 1, 3)
INSERT OR IGNORE INTO category_settings
    (category, display_name, language, tts_engine, voicevox_speaker_id, sort_order) VALUES
    ('tech',     'テックニュース',     'ja', 'voicevox', 3, 1),
    ('business', 'ビジネスニュース', 'ja', 'voicevox', 2, 2);

-- ── pipeline_runs: パイプライン実行履歴（依存なし）─────────────────
-- ※ cleanup対象外（意図的設計）。ホームサーバー規模では無限増殖しても実用上問題なし
CREATE TABLE IF NOT EXISTS pipeline_runs (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    job_type           TEXT    NOT NULL DEFAULT 'generate'
                               CHECK (job_type IN ('scrape', 'generate')), -- scrape=クロールのみ, generate=音声生成のみ
    status             TEXT    NOT NULL DEFAULT 'running'
                               CHECK (status IN ('running', 'completed', 'failed', 'cancelled')),
    triggered_by       TEXT    NOT NULL DEFAULT 'cron'
                               CHECK (triggered_by IN ('cron', 'api', 'ui')),
    current_step       TEXT,                             -- UI進捗表示用 ('scrape','select','tts','encode','store')
    articles_collected INTEGER,                          -- ScrapeJobが収集した件数
    articles_selected  INTEGER,                          -- GenerateJobがGeminiで選定した件数（0=処理対象なし）
    error_message      TEXT,
    started_at         TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    finished_at        TEXT
);
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_status     ON pipeline_runs (status);
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_started_at ON pipeline_runs (started_at DESC);

-- ── sources: スクレイピング対象ソース（依存なし）────────────────
CREATE TABLE IF NOT EXISTS sources (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT    NOT NULL,                       -- 表示名 (例: "Hacker News RSS")
    url          TEXT    NOT NULL UNIQUE,
    category     TEXT    NOT NULL REFERENCES category_settings(category),  -- 有効カテゴリはcategory_settingsで管理
    fetch_method TEXT    NOT NULL CHECK (fetch_method IN ('rss', 'http', 'playwright')),
    css_selector TEXT,                                   -- http/playwright用: 記事リンクCSSセレクタ
    enabled      INTEGER NOT NULL DEFAULT 1,             -- 0=無効, 1=有効
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
    -- ※ updated_at はSQLiteのON UPDATE非対応のため、source_repo.goのUpdate()で明示セット必須
    --    例: UPDATE sources SET name=?, ..., updated_at=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id=?
);
CREATE INDEX IF NOT EXISTS idx_sources_category ON sources (category);
CREATE INDEX IF NOT EXISTS idx_sources_enabled  ON sources (enabled);

-- ── broadcasts: 音声エピソード（pipeline_runsに依存）─────────────────
-- episode: 1パイプライン実行 × 1カテゴリ = 1レコード
-- digest:  全カテゴリをsort_order順に連結した合成エピソード = 1パイプライン実行に1レコード
CREATE TABLE IF NOT EXISTS broadcasts (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_run_id INTEGER NOT NULL REFERENCES pipeline_runs(id),
    category        TEXT    NOT NULL,        -- 'tech', 'business' 等 or 'digest'（全連結）
    broadcast_type  TEXT    NOT NULL DEFAULT 'episode'
                            CHECK (broadcast_type IN ('episode', 'digest')),
    title           TEXT    NOT NULL,        -- "Tech News - 2026-02-26 朝" 等
    script          TEXT,                    -- DJスタイル全文スクリプト（N記事連結, episodeのみ）
    file_path       TEXT    NOT NULL,        -- SMB共有内の相対パス（go-smb2での読み書き・削除キー）
                                             -- 例: ai-news/tech/tech-news-20260226-morning.mp3
                                             -- SMB_SHARE/{file_path} がSambaサーバー上の実体パス
    file_url        TEXT,                    -- GoアプリのHTTP URL (http://192.168.0.13:8181/media/tech/{id})
                                             -- APIレスポンスのmedia_urlとして返却。HAがmedia_content_idに使用。Web UIの<audio>タグのsrc経由でも使用
    duration_sec    INTEGER,                 -- 音声長さ（秒）: ffprobeで取得後にDB記録
    created_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_broadcasts_category   ON broadcasts (category);
CREATE INDEX IF NOT EXISTS idx_broadcasts_type       ON broadcasts (broadcast_type);
CREATE INDEX IF NOT EXISTS idx_broadcasts_created_at ON broadcasts (created_at DESC);

-- ── articles: 収集・処理済み記事（sources / pipeline_runs / broadcasts に依存）
CREATE TABLE IF NOT EXISTS articles (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id       INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    pipeline_run_id INTEGER REFERENCES pipeline_runs(id),  -- ScrapeJobのrun_id（記事を収集したScrapeジョブへの参照）
                                                           -- GenerateJob実行時は更新しない
    broadcast_id    INTEGER REFERENCES broadcasts(id) ON DELETE SET NULL,
                                                           -- ON DELETE SET NULL:
                                                           --   broadcasts削除時にNULLクリア（FK制約違反防止）
                                                           --   cleanup_usecase.goはbroadcast削除→article削除の順で実施
                                                           --   SQLiteが自動でNULLに更新するため明示的な前処理不要
    title           TEXT    NOT NULL,
    url             TEXT    NOT NULL UNIQUE,               -- 重複取り込み防止キー
    thumbnail_url   TEXT,                                  -- サムネール配信パス（例: /thumbnails/42.jpg）
                                                           -- nullable: リモート画像が取得できない記事は NULL
                                                           -- スクレイプ時に og:image / RSS media:thumbnail のURLを取得し
                                                           -- thumbnail.store.DownloadAndSave でリサイズ・クロップ後に
                                                           -- /data/thumbnails/{articleID}.jpg としてローカル保存
                                                           -- → DBには HTTPサービングパス "/thumbnails/{id}.jpg" を格納
                                                           -- → 記事削除時は thumbnail.store.Delete で実ファイルも削除必須
    raw_content     TEXT,                                  -- スクレイプ本文（処理後にNULLクリア可）
    category        TEXT    NOT NULL REFERENCES category_settings(category),  -- 有効カテゴリはcategory_settingsで管理
    is_selected     INTEGER NOT NULL DEFAULT 0,            -- 1=Geminiが選定したTop10
    select_rank     INTEGER,                               -- 話題性ランキング (1=最も高い〜10=最も低い, NULL=未選定)
    summary         TEXT,                                  -- 個別記事のDJ原稿（Gemini生成）
    created_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    processed_at    TEXT                                   -- broadcast_idが設定された時刻
);
CREATE INDEX IF NOT EXISTS idx_articles_category       ON articles (category);
CREATE INDEX IF NOT EXISTS idx_articles_is_selected    ON articles (is_selected);
CREATE INDEX IF NOT EXISTS idx_articles_created_at     ON articles (created_at);
CREATE INDEX IF NOT EXISTS idx_articles_pipeline_run   ON articles (pipeline_run_id);
CREATE INDEX IF NOT EXISTS idx_articles_broadcast      ON articles (broadcast_id);
CREATE INDEX IF NOT EXISTS idx_articles_select_rank    ON articles (select_rank);
