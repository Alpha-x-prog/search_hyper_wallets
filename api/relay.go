package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const relayBaseURL = "https://api.relay.link"

type RelayClient struct {
	http *http.Client
}

func NewRelayClient() *RelayClient {
	return &RelayClient{http: &http.Client{Timeout: 15 * time.Second}}
}

type RelayRequest struct {
	ID                 string `json:"id"`
	User               string `json:"user"`
	OriginChainId      int    `json:"originChainId"`
	DestinationChainId int    `json:"destinationChainId"`
	Status             string `json:"status"`
}

type RelayResponse struct {
	Requests []RelayRequest `json:"requests"`
}

// RelayResult — итог проверки кошелька через Relay
type RelayResult struct {
	Used        bool
	DestChainId int
}

// Check проверяет, использовал ли адрес Relay с Arbitrum (chainId=42161).
// Возвращает (relayUsed, destChainId, error).
func (c *RelayClient) Check(address string) (RelayResult, error) {
	url := fmt.Sprintf(
		"%s/requests/v2?user=%s&originChainId=42161",
		relayBaseURL, address,
	)

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

		var result RelayResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return RelayResult{}, fmt.Errorf("unmarshal: %w", err)
		}

		if len(result.Requests) == 0 {
			return RelayResult{Used: false}, nil
		}

		// берём первую (самую свежую) транзакцию
		return RelayResult{
			Used:        true,
			DestChainId: result.Requests[0].DestinationChainId,
		}, nil
	}
	return RelayResult{}, lastErr
}
