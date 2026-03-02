package main

// Phase 1 scaffold stub — Phase 2 で DI配線・HTTPサーバー起動を実装する
// 依存ライブラリのインポートパスを宣言して go mod tidy / go build を通す

import (
	_ "github.com/PuerkitoBio/goquery"
	_ "github.com/bogem/id3v2/v2"
	_ "github.com/disintegration/imaging"
	_ "github.com/hirochachacha/go-smb2"
	_ "github.com/joho/godotenv"
	_ "github.com/mmcdole/gofeed"
	_ "github.com/playwright-community/playwright-go"
	_ "github.com/robfig/cron/v3"
	_ "google.golang.org/genai"
	_ "modernc.org/sqlite"
)

func main() {}
