package main

import (
	"log"

	"arb_tracker/config"
	"arb_tracker/db"
	"arb_tracker/tracker"
)

func main() {
	cfg := config.Default()

	store, err := db.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("❌ Не удалось открыть БД: %v", err)
	}
	defer store.Close()

	t := tracker.New(cfg, store)
	t.Run()
}
