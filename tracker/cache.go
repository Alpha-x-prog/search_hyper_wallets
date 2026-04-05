package tracker

import "sync"

// SeenCache — потокобезопасный кэш уже обработанных хэшей транзакций
type SeenCache struct {
	mu   sync.Mutex
	seen map[string]bool
}

func NewSeenCache() *SeenCache {
	return &SeenCache{seen: make(map[string]bool)}
}

// TryAdd возвращает true, если хэш добавлен впервые (новый)
// Возвращает false, если хэш уже был добавлен ранее
func (s *SeenCache) TryAdd(hash string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[hash] {
		return false
	}
	s.seen[hash] = true
	return true
}
