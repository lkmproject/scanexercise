package config

import (
	"os"
	"strconv"
	"strings"
)

type Settings struct {
	TextInAppID          string
	TextInSecretCode     string
	DataDir              string
	MaxPages             int
	RetentionHours       int
	MaxLongEdge          int
	TextInTimeoutSeconds int
	MaxUploadBytes       int
	GoogleWebClientID    string
	AWSRegion            string
	JWTSecret            string
}

func Load() Settings {
	loadDotenv(".env")
	return Settings{
		TextInAppID:          getenv("TEXTIN_APP_ID", ""),
		TextInSecretCode:     getenv("TEXTIN_SECRET_CODE", ""),
		DataDir:              getenv("DATA_DIR", "data"),
		MaxPages:             getenvInt("MAX_PAGES", 10),
		RetentionHours:       getenvInt("RETENTION_HOURS", 24),
		MaxLongEdge:          getenvInt("MAX_LONG_EDGE", 2560),
		TextInTimeoutSeconds: getenvInt("TEXTIN_TIMEOUT_SECONDS", 180),
		MaxUploadBytes:       20 * 1024 * 1024,
		GoogleWebClientID: getenv(
			"GOOGLE_WEB_CLIENT_ID",
			"430840613406-4eki8kbemjbhc0jbah93v1vdc33fuf6p.apps.googleusercontent.com",
		),
		AWSRegion: getenv("AWS_REGION", "ap-southeast-1"),
		JWTSecret: getenv("JWT_SECRET", ""),
	}
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func loadDotenv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		stripped := strings.TrimSpace(line)
		if stripped == "" || strings.HasPrefix(stripped, "#") || !strings.Contains(stripped, "=") {
			continue
		}
		key, value, _ := strings.Cut(stripped, "=")
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
