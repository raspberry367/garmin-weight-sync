package config

import (
	"os"
	"path/filepath"

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
	}

	return cfg, nil
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
