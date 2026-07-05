// Command garmin-login performs a one-off interactive Garmin Connect login,
// including an MFA code prompt if Garmin requires one, and caches the
// resulting OAuth1 token pair to GARMIN_TOKEN_CACHE_PATH. Run this once
// before starting the server so the cron-based sync never has to handle an
// MFA challenge unattended.
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/rsb/garmin-weight-sync/config"
	"github.com/rsb/garmin-weight-sync/internal/adapter/garmin"
	"github.com/rsb/garmin-weight-sync/internal/adapter/telegram"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	notifier := telegram.New(cfg.TelegramBotToken, cfg.TelegramChatID)

	client, err := garmin.NewClient(garmin.Config{
		Username:       cfg.GarminUsername,
		Password:       cfg.GarminPassword,
		TokenCachePath: cfg.GarminTokenCachePath,
	})
	if err != nil {
		log.Fatalf("Failed to build garmin client: %v", err)
	}

	fmt.Println("Logging in to Garmin Connect...")
	err = client.LoginInteractive(context.Background(), func() (string, error) {
		fmt.Print("Enter MFA code: ")
		reader := bufio.NewReader(os.Stdin)
		code, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(code), nil
	})
	if err != nil {
		if nerr := notifier.Notify(context.Background(), fmt.Sprintf("⚠️ Garmin garmin-login failed: %v", err)); nerr != nil {
			log.Printf("failed to send alert: %v", nerr)
		}
		log.Fatalf("Login failed: %v", err)
	}

	fmt.Printf("Login succeeded. OAuth1 token cached at %s\n", cfg.GarminTokenCachePath)
}
