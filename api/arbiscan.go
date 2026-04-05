package api

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"

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

// get выполняет GET-запрос и десериализует JSON в out
func (c *Client) get(url string, out interface{}) error {
	resp, err := c.http.Get(url)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	return nil
}

// GetOutboundTxs возвращает USDC-переводы FROM bridgeAddress начиная с startBlock
func (c *Client) GetOutboundTxs(startBlock int64) ([]models.TokenTx, error) {
	url := fmt.Sprintf(
		"%s?module=account&action=tokentx&address=%s&contractaddress=%s&startblock=%d&sort=asc&apikey=%s",
		c.cfg.BaseURL,
		c.cfg.BridgeAddress,
		c.cfg.USDCAddress,
		startBlock,
		c.cfg.APIKey,
	)

	var result models.TokenTxResponse
	if err := c.get(url, &result); err != nil {
		return nil, err
	}

	if result.Status != "1" && result.Message != "No transactions found" {
		return nil, fmt.Errorf("arbiscan error: %s", result.Message)
	}

	// Фильтруем только исходящие от bridge
	var out []models.TokenTx
	for _, tx := range result.Result {
		if strings.EqualFold(tx.From, c.cfg.BridgeAddress) {
			out = append(out, tx)
		}
	}
	return out, nil
}

// GetBlockByTimestamp возвращает номер блока ближайший к переданному unix-timestamp
func (c *Client) GetBlockByTimestamp(ts int64) (int64, error) {
	url := fmt.Sprintf(
		"%s?module=block&action=getblocknobytime&timestamp=%d&closest=before&apikey=%s",
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
		"%s?module=account&action=tokenbalance&contractaddress=%s&address=%s&tag=latest&apikey=%s",
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
