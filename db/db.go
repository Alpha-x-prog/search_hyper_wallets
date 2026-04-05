package db

import (
	"database/sql"
	"fmt"
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
	TxTime  time.Time
	FoundAt time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS wallets (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	address  TEXT    NOT NULL,
	tx_hash  TEXT    NOT NULL UNIQUE,
	amount   REAL    NOT NULL,
	tx_time  DATETIME NOT NULL,
	found_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_address ON wallets(address);
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
	return &Store{conn: conn}, nil
}

func (s *Store) Close() {
	s.conn.Close()
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

// All возвращает все кошельки, отсортированные по времени TX
func (s *Store) All() ([]Wallet, error) {
	rows, err := s.conn.Query(`
		SELECT id, address, tx_hash, amount, tx_time, found_at
		FROM wallets ORDER BY tx_time DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wallets []Wallet
	for rows.Next() {
		var w Wallet
		if err := rows.Scan(&w.ID, &w.Address, &w.TxHash, &w.Amount, &w.TxTime, &w.FoundAt); err != nil {
			return nil, err
		}
		wallets = append(wallets, w)
	}
	return wallets, rows.Err()
}
