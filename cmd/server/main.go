package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/rsb/garmin-weight-sync/config"
	"github.com/rsb/garmin-weight-sync/internal/adapter/db"
	"github.com/rsb/garmin-weight-sync/internal/adapter/garmin"
	adapterhttp "github.com/rsb/garmin-weight-sync/internal/adapter/http"
	"github.com/rsb/garmin-weight-sync/internal/adapter/telegram"
	"github.com/rsb/garmin-weight-sync/internal/usecase"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to MySQL database with a retry mechanism
	var sqlDB *sql.DB
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)

	log.Printf("Connecting to database at %s:%s...", cfg.DBHost, cfg.DBPort)
	for i := 0; i < 10; i++ {
		sqlDB, err = sql.Open("mysql", dsn)
		if err == nil {
			err = sqlDB.Ping()
			if err == nil {
				break
			}
		}
		log.Printf("Database connection failed, retrying in 2 seconds... (attempt %d/10): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to connect to database after 10 attempts: %v", err)
	}

	// Configure database connection pooling limits
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	// Initialize MySQL Repository
	repo, err := db.NewMySQLRepository(sqlDB)
	if err != nil {
		sqlDB.Close()
		log.Fatalf("Failed to initialize repository: %v", err)
	}

	// Initialize Usecase
	syncUseCase := usecase.NewSyncMeasurementUseCase(repo)

	// Initialize Router with Usecase
	app := adapterhttp.SetupRouter(syncUseCase)

	// Initialize Garmin client + cron-based sync of unsynced measurements
	garminClient, err := garmin.NewClient(garmin.Config{
		Username:       cfg.GarminUsername,
		Password:       cfg.GarminPassword,
		TokenCachePath: cfg.GarminTokenCachePath,
	})
	if err != nil {
		log.Fatalf("Failed to initialize garmin client: %v", err)
	}
	notifier := telegram.New(cfg.TelegramBotToken, cfg.TelegramChatID)
	if notifier.Enabled() {
		log.Println("Telegram alerting enabled")
	}
	garminSyncUseCase := usecase.NewGarminSyncUseCase(repo, garminClient, notifier)

	syncCtx, stopSync := context.WithCancel(context.Background())
	syncDone := make(chan struct{})
	go runGarminSyncLoop(syncCtx, syncDone, garminSyncUseCase, time.Duration(cfg.SyncIntervalMinutes)*time.Minute)

	// Setup graceful shutdown listener
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Starting server on port :%s", cfg.ServerPort)
		if err := app.Listen(":" + cfg.ServerPort); err != nil && err != stdhttp.ErrServerClosed {
			log.Printf("Server failure: %v", err)
			sigChan <- syscall.SIGTERM
		}
	}()

	// Block until a signal is received
	sig := <-sigChan
	log.Printf("Received signal %v. Initiating graceful shutdown...", sig)

	// Stop the sync loop and wait for any in-flight sync to finish
	stopSync()
	<-syncDone

	// Shutdown Fiber app with a short timeout
	if err := app.Shutdown(); err != nil {
		log.Printf("Fiber server shutdown error: %v", err)
	}

	// Close database connections pool
	if err := sqlDB.Close(); err != nil {
		log.Printf("Database connection pool close error: %v", err)
	}

	log.Println("Shutdown complete. Exiting.")
}

// runGarminSyncLoop runs an immediate sync, then one every interval, until
// ctx is cancelled. It closes done once the loop has fully stopped so the
// caller can wait out any sync in progress before tearing down the DB pool.
func runGarminSyncLoop(ctx context.Context, done chan<- struct{}, useCase *usecase.GarminSyncUseCase, interval time.Duration) {
	defer close(done)

	runSync := func() {
		if err := useCase.Execute(ctx); err != nil {
			log.Printf("Garmin sync failed: %v", err)
		}
	}

	runSync()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSync()
		}
	}
}
