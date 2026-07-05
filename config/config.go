package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	ServerPort     string `env:"SERVER_PORT" envDefault:"3000"`
	GarminUsername string `env:"GARMIN_USERNAME" required:"true"`
	GarminPassword string `env:"GARMIN_PASSWORD" required:"true"`
	APIKey         string `env:"API_KEY" required:"true"`
	DBHost         string `env:"DB_HOST" envDefault:"localhost"`
	DBPort         string `env:"DB_PORT" envDefault:"3306"`
	DBUser         string `env:"DB_USER" envDefault:"appuser"`
	DBPassword     string `env:"DB_PASSWORD" envDefault:"apppass"`
	DBName         string `env:"DB_NAME" envDefault:"garmin_weight_sync"`

	GarminTokenCachePath string `env:"GARMIN_TOKEN_CACHE_PATH" envDefault:"garmin_token.json"`
	SyncIntervalMinutes  int    `env:"SYNC_INTERVAL_MINUTES" envDefault:"60"`

	// Optional Telegram alerting. When either is empty, alerts are disabled.
	TelegramBotToken string `env:"TELEGRAM_BOT_TOKEN"`
	TelegramChatID   string `env:"TELEGRAM_CHAT_ID"`
}

// NewConfig creates a new Config instance by loading from environment.
func NewConfig() (*Config, error) {
	if err := loadDotEnv(); err != nil {
		return nil, err
	}

	cfg := &Config{
		ServerPort:     getEnv("SERVER_PORT", "3000"),
		GarminUsername: getEnv("GARMIN_USERNAME", ""),
		GarminPassword: getEnv("GARMIN_PASSWORD", ""),
		APIKey:         getEnv("API_KEY", ""),
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "3306"),
		DBUser:         getEnv("DB_USER", "appuser"),
		DBPassword:     getEnv("DB_PASSWORD", "apppass"),
		DBName:         getEnv("DB_NAME", "garmin_weight_sync"),

		GarminTokenCachePath: getEnv("GARMIN_TOKEN_CACHE_PATH", "garmin_token.json"),
		SyncIntervalMinutes:  getEnvInt("SYNC_INTERVAL_MINUTES", 60),

		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:   getEnv("TELEGRAM_CHAT_ID", ""),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate enforces the required env vars and sane value ranges at boot,
// rather than failing later inside the background sync goroutine (or, for
// SyncIntervalMinutes, panicking time.NewTicker at startup).
func (c *Config) validate() error {
	if c.GarminUsername == "" {
		return fmt.Errorf("GARMIN_USERNAME is required")
	}
	if c.GarminPassword == "" {
		return fmt.Errorf("GARMIN_PASSWORD is required")
	}
	if c.APIKey == "" {
		return fmt.Errorf("API_KEY is required")
	}
	if c.SyncIntervalMinutes <= 0 {
		return fmt.Errorf("SYNC_INTERVAL_MINUTES must be positive, got %d", c.SyncIntervalMinutes)
	}
	return nil
}

func loadDotEnv() error {
	paths := []string{".env", filepath.Join(".env.local")}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			values, err := godotenv.Read(path)
			if err != nil {
				return err
			}
			for key, value := range values {
				if _, exists := os.LookupEnv(key); !exists || os.Getenv(key) == "" {
					if err := os.Setenv(key, value); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return defaultValue
	}
	return parsed
}
