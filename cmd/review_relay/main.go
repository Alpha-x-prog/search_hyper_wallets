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

	relay := api.NewRelayClient()

	candidates, err := store.CandidatesWithoutRelay()
	if err != nil {
		log.Fatalf("❌ Чтение кандидатов: %v", err)
	}

	if len(candidates) == 0 {
		log.Println("✅ Все кандидаты уже проверены через Relay")
		return
	}

	log.Printf("🔍 Кандидатов без relay-проверки: %d", len(candidates))

	relayUsed := 0
	relayNot := 0

	for i, c := range candidates {
		log.Printf("[%d/%d] %s | balance=$%.2f | txCount=%d",
			i+1, len(candidates), c.Address, c.Balance, c.TxCount)

		time.Sleep(2 * time.Second)

		result, err := relay.Check(c.Address)
		if err != nil {
			log.Printf("  ⚠️  Relay error: %v", err)
			continue
		}

		var destChainId *int
		if result.Used {
			destChainId = &result.DestChainId
		}

		if err := store.SaveRelayResult(c.Address, result.Used, destChainId); err != nil {
			log.Printf("  ❌ SaveRelayResult: %v", err)
			continue
		}

		if result.Used {
			log.Printf("  🌉 Relay ИСПОЛЬЗОВАН | destChain=%d", result.DestChainId)
			relayUsed++
		} else {
			log.Printf("  🔕 Relay не использован")
			relayNot++
		}
	}

	log.Printf("\n✅ Готово: relay использован=%d | не использован=%d", relayUsed, relayNot)
}
