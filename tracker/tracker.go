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
	relay  *api.RelayClient
	store  *db.Store
	cache  *SeenCache
	cfg    *config.Config
}

func New(cfg *config.Config, store *db.Store) *Tracker {
	return &Tracker{
		client: api.NewClient(cfg),
		relay:  api.NewRelayClient(),
		store:  store,
		cache:  NewSeenCache(),
		cfg:    cfg,
	}
}

// Run запускает ресёрч (или продолжает с last_block), затем входит в поллинг-цикл
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

// research — при первом запуске сканирует 48ч, при повторном — продолжает с last_block
func (t *Tracker) research() int64 {
	saved := t.store.GetLastBlock()
	if saved > 0 {
		log.Printf("♻️  Продолжаем с блока %d (сохранён в БД)", saved)
		total, _ := t.store.Count()
		log.Printf("📦 Уже в БД: %d кошельков", total)
		return saved
	}

	since := time.Now().Add(-t.cfg.ResearchPeriod)
	log.Printf("🔍 Первый запуск — ресёрч за %v (с %s)...",
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

	if lastBlock > 0 {
		t.store.SetLastBlock(lastBlock)
	}

	total, _ := t.store.Count()
	log.Printf("✅ Ресёрч завершён: %d новых хитов | Всего в БД: %d", hits, total)
	return lastBlock
}

// poll — регулярная проверка новых TX, сохраняет last_block после каждого цикла
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

	if lastBlock > 0 {
		t.store.SetLastBlock(lastBlock)
	}

	return lastBlock
}

// save записывает хит в БД и запускает горутину проверки баланса через 1 минуту
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

	go t.checkBalance(hit.Tx.To, hit.Tx.Hash)
}

// checkBalance — горутина: ждёт 1 мин, затем последовательно проверяет
// баланс → кол-во TX → relay. Логирует каждый шаг.
func (t *Tracker) checkBalance(address, txHash string) {
	log.Printf("⏳ [1/4] Ждём 1 мин перед проверкой | %s", address)
	time.Sleep(1 * time.Minute)

	// Шаг 1: баланс
	log.Printf("🔎 [2/4] Проверяем баланс | %s", address)
	balance, err := t.client.GetUSDCBalance(address)
	if err != nil {
		log.Printf("⚠️  Баланс ошибка | %s | %v", address, err)
		return
	}
	if err := t.store.UpdateBalance(txHash, balance); err != nil {
		log.Printf("❌ UpdateBalance | %s | %v", address, err)
		return
	}
	log.Printf("💰 Баланс | %s | $%.2f USDC", address, balance)

	if balance >= 10 {
		log.Printf("🚫 Пропуск (balance=$%.2f >= $10) | %s", balance, address)
		return
	}

	// Шаг 2: количество транзакций
	log.Printf("🔎 [3/4] Проверяем кол-во TX | %s", address)
	txCount, err := t.client.GetTxCount(address)
	if err != nil {
		log.Printf("⚠️  TxCount ошибка | %s | %v", address, err)
		return
	}
	log.Printf("📊 TxCount | %s | %d транзакций", address, txCount)

	if txCount >= 8 {
		log.Printf("🚫 Пропуск (txCount=%d >= 8) | %s", txCount, address)
		return
	}

	// Сохраняем кандидата
	if err := t.store.SaveCandidate(address, balance, txCount); err != nil {
		log.Printf("❌ SaveCandidate | %s | %v", address, err)
		return
	}
	log.Printf("🎯 Кандидат сохранён | %s | balance=$%.2f | txCount=%d", address, balance, txCount)

	// Шаг 3: relay
	log.Printf("🔎 [4/4] Проверяем Relay | %s", address)
	relayRes, err := t.relay.Check(address)
	if err != nil {
		log.Printf("⚠️  Relay ошибка | %s | %v", address, err)
		return
	}

	var destChainId *int
	if relayRes.Used {
		destChainId = &relayRes.DestChainId
	}

	if err := t.store.SaveRelayResult(address, relayRes.Recipient, relayRes.Used, destChainId); err != nil {
		log.Printf("❌ SaveRelayResult | %s | %v", address, err)
		return
	}

	if relayRes.Used {
		log.Printf("🌉 Relay ИСПОЛЬЗОВАН | %s | recipient=%s | destChain=%d",
			address, relayRes.Recipient, relayRes.DestChainId)
	} else {
		log.Printf("🔕 Relay не использован | %s", address)
	}
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
