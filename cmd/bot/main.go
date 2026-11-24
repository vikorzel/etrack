package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	tgbot "github.com/go-telegram/bot"

	mealbot "meal-bot/internal/bot"
	"meal-bot/internal/config"
	"meal-bot/internal/storage"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	loadCtx, cancelLoad := context.WithTimeout(ctx, 15*time.Second)
	defer cancelLoad()

	cfg, err := config.Load(loadCtx, ".env")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	store, err := storage.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("open sqlite store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close sqlite store: %v", err)
		}
	}()

	handler := mealbot.NewHandler(store, cfg.Location)

	b, err := tgbot.New(cfg.Token, tgbot.WithDefaultHandler(handler.DefaultHandler()))
	if err != nil {
		log.Fatalf("create telegram bot: %v", err)
	}

	log.Printf("meal-bot started (version=%s, source=%s, db=%s, timezone=%s)", version, cfg.Source, cfg.DBPath, cfg.Location.String())
	b.Start(ctx)
}
