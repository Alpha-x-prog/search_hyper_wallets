package db

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

// Store — обёртка над SQLite
type Store struct {
	conn *sql.DB
}

// Wallet — одна запись в БД
type Wallet struct {
	ID      int64
	Address string
	TxHash  string
	Amount  float64
	Balance *float64
	TxTime  time.Time
	FoundAt time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS wallets (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	address  TEXT    NOT NULL,
	tx_hash  TEXT    NOT NULL UNIQUE,
	amount   REAL    NOT NULL,
	balance  REAL,
	tx_time  DATETIME NOT NULL,
	found_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS candidates (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	address   TEXT    NOT NULL UNIQUE,
	balance   REAL    NOT NULL,
	tx_count  INTEGER NOT NULL,
	found_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS relay_results (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	address       TEXT    NOT NULL UNIQUE,
	relay_used    INTEGER NOT NULL DEFAULT 0,
	dest_chain_id INTEGER,
	checked_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_address  ON wallets(address);
CREATE INDEX IF NOT EXISTS idx_found_at ON wallets(found_at);
`

func New(path string) (*Store, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := conn.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	// мягкая миграция: добавляем колонки/таблицы если их нет
	conn.Exec(`ALTER TABLE wallets ADD COLUMN balance REAL`)
	return &Store{conn: conn}, nil
}

func (s *Store) Close() {
	s.conn.Close()
}

// GetLastBlock возвращает последний сохранённый блок (0 если не сохранён)
func (s *Store) GetLastBlock() int64 {
	var val string
	err := s.conn.QueryRow(`SELECT value FROM settings WHERE key = 'last_block'`).Scan(&val)
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(val, 10, 64)
	return n
}

// SetLastBlock сохраняет последний обработанный блок
func (s *Store) SetLastBlock(block int64) error {
	_, err := s.conn.Exec(
		`INSERT INTO settings(key, value) VALUES('last_block', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		strconv.FormatInt(block, 10),
	)
	return err
}

// Save сохраняет найденный кошелёк. Если tx_hash уже есть — молча игнорирует (дубль).
func (s *Store) Save(address, txHash string, amount float64, txTime time.Time) error {
	_, err := s.conn.Exec(`
		INSERT OR IGNORE INTO wallets (address, tx_hash, amount, tx_time, found_at)
		VALUES (?, ?, ?, ?, ?)`,
		address, txHash, amount, txTime.UTC(), time.Now().UTC(),
	)
	return err
}

// WithoutBalance возвращает все записи где balance IS NULL
func (s *Store) WithoutBalance() ([]Wallet, error) {
	rows, err := s.conn.Query(`
		SELECT id, address, tx_hash, amount, balance, tx_time, found_at
		FROM wallets WHERE balance IS NULL ORDER BY found_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wallets []Wallet
	for rows.Next() {
		var w Wallet
		if err := rows.Scan(&w.ID, &w.Address, &w.TxHash, &w.Amount, &w.Balance, &w.TxTime, &w.FoundAt); err != nil {
			return nil, err
		}
		wallets = append(wallets, w)
	}
	return wallets, rows.Err()
}

// UpdateBalance обновляет баланс USDC для записи по tx_hash
func (s *Store) UpdateBalance(txHash string, balance float64) error {
	_, err := s.conn.Exec(`UPDATE wallets SET balance = ? WHERE tx_hash = ?`, balance, txHash)
	return err
}

// Exists проверяет, есть ли уже такой tx_hash в БД
func (s *Store) Exists(txHash string) (bool, error) {
	var count int
	err := s.conn.QueryRow(`SELECT COUNT(*) FROM wallets WHERE tx_hash = ?`, txHash).Scan(&count)
	return count > 0, err
}

// Count возвращает общее количество сохранённых кошельков
func (s *Store) Count() (int64, error) {
	var n int64
	err := s.conn.QueryRow(`SELECT COUNT(*) FROM wallets`).Scan(&n)
	return n, err
}

// Candidate — запись в таблице кандидатов
type Candidate struct {
	ID      int64
	Address string
	Balance float64
	TxCount int
	FoundAt time.Time
}

// SaveCandidate сохраняет кандидата. Если address уже есть — игнорирует.
func (s *Store) SaveCandidate(address string, balance float64, txCount int) error {
	_, err := s.conn.Exec(`
		INSERT OR IGNORE INTO candidates (address, balance, tx_count, found_at)
		VALUES (?, ?, ?, ?)`,
		address, balance, txCount, time.Now().UTC(),
	)
	return err
}

// CandidateExists проверяет, есть ли адрес в candidates
func (s *Store) CandidateExists(address string) (bool, error) {
	var count int
	err := s.conn.QueryRow(`SELECT COUNT(*) FROM candidates WHERE address = ?`, address).Scan(&count)
	return count > 0, err
}

// AllCandidates возвращает всех кандидатов
func (s *Store) AllCandidates() ([]Candidate, error) {
	rows, err := s.conn.Query(`
		SELECT id, address, balance, tx_count, found_at
		FROM candidates ORDER BY found_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ID, &c.Address, &c.Balance, &c.TxCount, &c.FoundAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// RelayResult — запись в таблице relay_results
type RelayResult struct {
	ID          int64
	Address     string
	RelayUsed   bool
	DestChainId *int
	CheckedAt   time.Time
}

// SaveRelayResult сохраняет результат проверки relay. Если адрес уже есть — игнорирует.
func (s *Store) SaveRelayResult(address string, relayUsed bool, destChainId *int) error {
	used := 0
	if relayUsed {
		used = 1
	}
	_, err := s.conn.Exec(`
		INSERT OR IGNORE INTO relay_results (address, relay_used, dest_chain_id, checked_at)
		VALUES (?, ?, ?, ?)`,
		address, used, destChainId, time.Now().UTC(),
	)
	return err
}

// RelayResultExists проверяет, есть ли адрес в relay_results
func (s *Store) RelayResultExists(address string) (bool, error) {
	var count int
	err := s.conn.QueryRow(`SELECT COUNT(*) FROM relay_results WHERE address = ?`, address).Scan(&count)
	return count > 0, err
}

// CandidatesWithoutRelay возвращает кандидатов, которых нет в relay_results
func (s *Store) CandidatesWithoutRelay() ([]Candidate, error) {
	rows, err := s.conn.Query(`
		SELECT c.id, c.address, c.balance, c.tx_count, c.found_at
		FROM candidates c
		LEFT JOIN relay_results r ON c.address = r.address
		WHERE r.address IS NULL
		ORDER BY c.found_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ID, &c.Address, &c.Balance, &c.TxCount, &c.FoundAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// All возвращает все кошельки, отсортированные по времени TX
func (s *Store) All() ([]Wallet, error) {
	rows, err := s.conn.Query(`
		SELECT id, address, tx_hash, amount, balance, tx_time, found_at
		FROM wallets ORDER BY tx_time DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wallets []Wallet
	for rows.Next() {
		var w Wallet
		if err := rows.Scan(&w.ID, &w.Address, &w.TxHash, &w.Amount, &w.Balance, &w.TxTime, &w.FoundAt); err != nil {
			return nil, err
		}
		wallets = append(wallets, w)
	}
	return wallets, rows.Err()
}
