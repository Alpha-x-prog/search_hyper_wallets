package api

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"arb_tracker/config"
	"arb_tracker/models"
)

// Client — HTTP-клиент для Arbiscan API
type Client struct {
	cfg  *config.Config
	http *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{},
	}
}

// get выполняет GET-запрос и десериализует JSON в out (3 попытки при сетевых ошибках)
func (c *Client) get(url string, out interface{}) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := c.http.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("http get: %w", err)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read body: %w", err)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}

		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}
		return nil
	}
	return lastErr
}

// GetOutboundTxs возвращает USDC-переводы FROM bridgeAddress начиная с startBlock
func (c *Client) GetOutboundTxs(startBlock int64) ([]models.TokenTx, error) {
	url := fmt.Sprintf(
		"%s?chainid=42161&module=account&action=tokentx&address=%s&contractaddress=%s&startblock=%d&sort=asc&apikey=%s",
		c.cfg.BaseURL,
		c.cfg.BridgeAddress,
		c.cfg.USDCAddress,
		startBlock,
		c.cfg.APIKey,
	)

	var raw struct {
		Status  string          `json:"status"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := c.get(url, &raw); err != nil {
		return nil, err
	}

	if raw.Status != "1" {
		if raw.Message == "No transactions found" {
			return nil, nil
		}
		// result может быть строкой с описанием ошибки
		var errMsg string
		_ = json.Unmarshal(raw.Result, &errMsg)
		if errMsg != "" {
			return nil, fmt.Errorf("arbiscan error: %s", errMsg)
		}
		return nil, fmt.Errorf("arbiscan error: %s", raw.Message)
	}

	var txs []models.TokenTx
	if err := json.Unmarshal(raw.Result, &txs); err != nil {
		return nil, fmt.Errorf("unmarshal txs: %w", err)
	}

	// Фильтруем только исходящие от bridge
	var out []models.TokenTx
	for _, tx := range txs {
		if strings.EqualFold(tx.From, c.cfg.BridgeAddress) {
			out = append(out, tx)
		}
	}
	return out, nil
}

// GetBlockByTimestamp возвращает номер блока ближайший к переданному unix-timestamp
func (c *Client) GetBlockByTimestamp(ts int64) (int64, error) {
	url := fmt.Sprintf(
		"%s?chainid=42161&module=block&action=getblocknobytime&timestamp=%d&closest=before&apikey=%s",
		c.cfg.BaseURL, ts, c.cfg.APIKey,
	)
	var result struct {
		Status string `json:"status"`
		Result string `json:"result"`
	}
	if err := c.get(url, &result); err != nil {
		return 0, err
	}
	block, err := strconv.ParseInt(result.Result, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad block number: %s", result.Result)
	}
	return block, nil
}

// GetUSDCBalance возвращает баланс USDC адреса в человекочитаемом виде
func (c *Client) GetUSDCBalance(address string) (float64, error) {
	url := fmt.Sprintf(
		"%s?chainid=42161&module=account&action=tokenbalance&contractaddress=%s&address=%s&tag=latest&apikey=%s",
		c.cfg.BaseURL,
		c.cfg.USDCAddress,
		address,
		c.cfg.APIKey,
	)

	var result models.BalanceResponse
	if err := c.get(url, &result); err != nil {
		return 0, err
	}

	return parseTokenAmount(result.Result, 6)
}

// GetTxCount возвращает количество обычных транзакций кошелька (до 10).
// Нам важно только знать < 8 или нет — поэтому тянем максимум 10 штук.
func (c *Client) GetTxCount(address string) (int, error) {
	url := fmt.Sprintf(
		"%s?chainid=42161&module=account&action=txlist&address=%s&page=1&offset=10&sort=asc&apikey=%s",
		c.cfg.BaseURL, address, c.cfg.APIKey,
	)
	var raw struct {
		Status  string          `json:"status"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := c.get(url, &raw); err != nil {
		return 0, err
	}
	if raw.Status != "1" {
		if raw.Message == "No transactions found" {
			return 0, nil
		}
		var errMsg string
		_ = json.Unmarshal(raw.Result, &errMsg)
		if errMsg != "" {
			return 0, fmt.Errorf("arbiscan error: %s", errMsg)
		}
		return 0, fmt.Errorf("arbiscan error: %s", raw.Message)
	}
	var txs []json.RawMessage
	if err := json.Unmarshal(raw.Result, &txs); err != nil {
		return 0, fmt.Errorf("unmarshal txs: %w", err)
	}
	return len(txs), nil
}

// ParseTxAmount переводит сырое значение токена в float64 с учётом decimals
func ParseTxAmount(value, decimalsStr string) (float64, error) {
	decimals := int64(6)
	if decimalsStr != "" {
		fmt.Sscanf(decimalsStr, "%d", &decimals)
	}
	return parseTokenAmount(value, decimals)
}

func parseTokenAmount(raw string, decimals int64) (float64, error) {
	n, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return 0, fmt.Errorf("invalid token amount: %s", raw)
	}
	divisor := new(big.Float).SetInt(
		new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil),
	)
	result, _ := new(big.Float).Quo(new(big.Float).SetInt(n), divisor).Float64()
	return result, nil
}
