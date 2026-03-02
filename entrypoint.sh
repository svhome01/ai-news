#!/bin/sh
# entrypoint.sh — 起動時パーミッション修正 → アプリ起動
set -e
# /data の所有者を現在のユーザーに変更（bind mountでroot所有になっている場合の対策）
# SQLite WALファイルの書き込み権限を確保するために必要
chown -R "$(id -u):$(id -g)" /data 2>/dev/null || true
exec "$@"
