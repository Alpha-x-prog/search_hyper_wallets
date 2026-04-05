package tracker

import (
	"log"
	"strconv"
	"time"

	"arb_tracker/api"
	"arb_tracker/config"
	"arb_tracker/db"
	"arb_tracker/models"
)

// Tracker — основной поллинг-цикл
type Tracker struct {
	client *api.Client
	store  *db.Store
	cache  *SeenCache
	cfg    *config.Config
}

func New(cfg *config.Config, store *db.Store) *Tracker {
	return &Tracker{
		client: api.NewClient(cfg),
		store:  store,
		cache:  NewSeenCache(),
		cfg:    cfg,
	}
}

// Run запускает ресёрч за 24ч, затем входит в поллинг-цикл
func (t *Tracker) Run() {
	log.Println("🚀 Arbitrum USDC Tracker запущен")
	log.Printf("📍 Bridge:  %s", t.cfg.BridgeAddress)
	log.Printf("💵 Фильтр: $%.0f – $%.0f USDC", t.cfg.MinUSD, t.cfg.MaxUSD)
	log.Printf("🗄  БД:     %s", t.cfg.DBPath)

	lastBlock := t.research()

	log.Printf("👀 Мониторинг новых TX каждые %v...\n", t.cfg.PollInterval)

	ticker := time.NewTicker(t.cfg.PollInterval)
	defer ticker.Stop()

	for range ticker.C {
		lastBlock = t.poll(lastBlock)
	}
}

// research — разовый прогон истории за ResearchPeriod при старте
func (t *Tracker) research() int64 {
	since := time.Now().Add(-t.cfg.ResearchPeriod)
	log.Printf("🔍 Ресёрч за последние %v (с %s)...",
		t.cfg.ResearchPeriod, since.Format("02 Jan 15:04"))

	startBlock, err := t.client.GetBlockByTimestamp(since.Unix())
	if err != nil {
		log.Printf("⚠️  Не удалось получить стартовый блок: %v", err)
		startBlock = 0
	} else {
		log.Printf("📦 Стартовый блок: %d", startBlock)
	}

	txs, err := t.client.GetOutboundTxs(startBlock)
	if err != nil {
		log.Printf("⚠️  Ошибка загрузки TX: %v", err)
		return startBlock
	}

	log.Printf("📊 Найдено %d TX за период, фильтруем...", len(txs))

	var lastBlock int64
	hits := 0
	for _, tx := range txs {
		t.cache.TryAdd(tx.Hash)
		if b := parseBlock(tx.BlockNumber); b > lastBlock {
			lastBlock = b
		}
		if hit, ok := t.applyFilter(tx); ok {
			t.save(hit)
			hits++
		}
	}

	total, _ := t.store.Count()
	log.Printf("✅ Ресёрч завершён: %d новых хитов | Всего в БД: %d", hits, total)
	return lastBlock
}

// poll — регулярная проверка новых TX
func (t *Tracker) poll(lastBlock int64) int64 {
	txs, err := t.client.GetOutboundTxs(lastBlock)
	if err != nil {
		log.Printf("❌ API ошибка: %v", err)
		return lastBlock
	}

	for _, tx := range txs {
		if b := parseBlock(tx.BlockNumber); b > lastBlock {
			lastBlock = b
		}
		if !t.cache.TryAdd(tx.Hash) {
			continue
		}
		if hit, ok := t.applyFilter(tx); ok {
			t.save(hit)
		}
	}

	return lastBlock
}

// save записывает хит в БД и выводит лог
func (t *Tracker) save(hit models.Hit) {
	ts, _ := strconv.ParseInt(hit.Tx.TimeStamp, 10, 64)
	txTime := time.Unix(ts, 0)

	if err := t.store.Save(hit.Tx.To, hit.Tx.Hash, hit.Amount, txTime); err != nil {
		log.Printf("❌ Ошибка сохранения %s: %v", hit.Tx.Hash, err)
		return
	}

	total, _ := t.store.Count()
	log.Printf("💾 Сохранён | %s | %s | $%.0f USDC | Всего в БД: %d",
		txTime.Format("02 Jan 15:04:05"), hit.Tx.To, hit.Amount, total)
}

// applyFilter проверяет диапазон суммы
func (t *Tracker) applyFilter(tx models.TokenTx) (models.Hit, bool) {
	amount, err := api.ParseTxAmount(tx.Value, tx.TokenDecimal)
	if err != nil {
		return models.Hit{}, false
	}
	if amount < t.cfg.MinUSD || amount > t.cfg.MaxUSD {
		return models.Hit{}, false
	}
	return models.Hit{Tx: tx, Amount: amount}, true
}

func parseBlock(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}
