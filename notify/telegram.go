package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Telegram отправляет сообщения в канал через Bot API (без сторонних библиотек)
type Telegram struct {
	token     string
	channelID int64
	http      *http.Client
}

func NewTelegram(token string, channelID int64) *Telegram {
	return &Telegram{
		token:     token,
		channelID: channelID,
		http:      &http.Client{Timeout: 10 * time.Second},
	}
}

type sendMessageBody struct {
	ChatID    int64  `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// Send отправляет HTML-сообщение в канал. 3 попытки при ошибке.
func (t *Telegram) Send(text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)

	body := sendMessageBody{
		ChatID:    t.channelID,
		Text:      text,
		ParseMode: "HTML",
	}

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := t.http.Post(url, "application/json", bytes.NewReader(data))
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return nil
		}
		lastErr = fmt.Errorf("telegram status: %d", resp.StatusCode)
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	return lastErr
}

// chainName возвращает название сети по chainId
func ChainName(chainId int) string {
	names := map[int]string{
		1:     "Ethereum",
		10:    "Optimism",
		56:    "BSC",
		137:   "Polygon",
		8453:  "Base",
		42161: "Arbitrum",
		43114: "Avalanche",
		59144: "Linea",
		534352: "Scroll",
	}
	if name, ok := names[chainId]; ok {
		return name
	}
	return fmt.Sprintf("Chain %d", chainId)
}
