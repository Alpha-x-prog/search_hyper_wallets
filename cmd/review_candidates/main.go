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

	wallets, err := store.All()
	if err != nil {
		log.Fatalf("❌ Чтение БД: %v", err)
	}

	log.Printf("🔍 Всего кошельков в БД: %d", len(wallets))

	added := 0
	skipped := 0

	for i, w := range wallets {
		log.Printf("[%d/%d] %s", i+1, len(wallets), w.Address)

		// Проверяем, уже есть ли в candidates
		exists, err := store.CandidateExists(w.Address)
		if err != nil {
			log.Printf("  ⚠️  CandidateExists: %v", err)
			continue
		}
		if exists {
			log.Printf("  ⏭️  Уже в candidates, пропускаем")
			skipped++
			continue
		}

		time.Sleep(3 * time.Second)

		balance, err := client.GetUSDCBalance(w.Address)
		if err != nil {
			log.Printf("  ⚠️  Баланс: %v", err)
			continue
		}

		if balance >= 10 {
			log.Printf("  💰 Баланс $%.2f — не подходит (>= $10)", balance)
			continue
		}

		time.Sleep(3 * time.Second)

		txCount, err := client.GetTxCount(w.Address)
		if err != nil {
			log.Printf("  ⚠️  TxCount: %v", err)
			continue
		}

		if txCount >= 8 {
			log.Printf("  📊 TxCount %d — не подходит (>= 8)", txCount)
			continue
		}

		if err := store.SaveCandidate(w.Address, balance, txCount); err != nil {
			log.Printf("  ❌ SaveCandidate: %v", err)
			continue
		}

		log.Printf("  🎯 ДОБАВЛЕН | balance=$%.2f | txCount=%d", balance, txCount)
		added++
	}

	total, _ := store.AllCandidates()
	log.Printf("\n✅ Готово: добавлено %d | пропущено %d | всего кандидатов: %d", added, skipped, len(total))
}
