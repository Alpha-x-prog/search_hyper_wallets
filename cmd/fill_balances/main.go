package main

import (
	"log"
	"time"

	"arb_tracker/api"
	"arb_tracker/config"
	"arb_tracker/db"
)

func main() {
	cfg := config.Default()

	store, err := db.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("❌ Не удалось открыть БД: %v", err)
	}
	defer store.Close()

	client := api.NewClient(cfg)

	log.Println("🔄 Balance filler запущен — интервал 3 сек/кошелёк")

	for {
		wallets, err := store.WithoutBalance()
		if err != nil {
			log.Printf("❌ Ошибка чтения БД: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if len(wallets) == 0 {
			log.Println("✅ Все балансы заполнены. Жду 30 сек...")
			time.Sleep(30 * time.Second)
			continue
		}

		log.Printf("📋 Осталось заполнить: %d кошельков", len(wallets))

		for _, w := range wallets {
			time.Sleep(3 * time.Second)

			balance, err := client.GetUSDCBalance(w.Address)
			if err != nil {
				log.Printf("⚠️  %s: %v", w.Address, err)
				continue
			}

			if err := store.UpdateBalance(w.TxHash, balance); err != nil {
				log.Printf("❌ Обновление %s: %v", w.Address, err)
				continue
			}

			log.Printf("💰 %s | $%.2f USDC", w.Address, balance)
		}
	}
}
