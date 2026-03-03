package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Port                  string
	DBPath                string
	GeminiAPIKey          string
	MaxGeminiConcurrency  int
	VOICEVOXUrl           string
	PlaywrightCDPEndpoint string
	NavidromeURL          string
	NavidromeUser         string
	NavidromePass         string
	SMBHost               string
	SMBUser               string
	SMBPass               string
	SMBShare              string
	SMBMusicPath          string
	AppBaseURL            string
	TZ                    string
}

// Load reads environment variables (and optionally .env) into a Config.
// .env loading is best-effort; missing file is not an error.
func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		Port:                  getEnv("PORT", "8181"),
		DBPath:                getEnv("DB_PATH", "/data/news.db"),
		GeminiAPIKey:          getEnv("GEMINI_API_KEY", ""),
		MaxGeminiConcurrency:  getEnvInt("MAX_GEMINI_CONCURRENCY", 2),
		VOICEVOXUrl:           getEnv("VOICEVOX_URL", "http://voicevox:50021"),
		PlaywrightCDPEndpoint: getEnv("PLAYWRIGHT_CDP_ENDPOINT", "ws://playwright-chrome:3000"),
		NavidromeURL:          getEnv("NAVIDROME_URL", "http://192.168.0.23:4533"),
		NavidromeUser:         getEnv("NAVIDROME_USER", ""),
		NavidromePass:         getEnv("NAVIDROME_PASS", ""),
		SMBHost:               getEnv("SMB_HOST", "192.168.0.22"),
		SMBUser:               getEnv("SMB_USER", ""),
		SMBPass:               getEnv("SMB_PASS", ""),
		SMBShare:              getEnv("SMB_SHARE", "Music"),
		SMBMusicPath:          getEnv("SMB_MUSIC_PATH", "ai-news"),
		AppBaseURL:            getEnv("APP_BASE_URL", "http://192.168.0.13:8181"),
		TZ:                    getEnv("TZ", "Asia/Tokyo"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}
