package models

// TokenTx — одна ERC-20 транзакция из Arbiscan
type TokenTx struct {
	BlockNumber  string `json:"blockNumber"`
	TimeStamp    string `json:"timeStamp"`
	Hash         string `json:"hash"`
	From         string `json:"from"`
	To           string `json:"to"`
	Value        string `json:"value"`
	TokenSymbol  string `json:"tokenSymbol"`
	TokenDecimal string `json:"tokenDecimal"`
}

// TokenTxResponse — ответ Arbiscan на запрос tokentx
type TokenTxResponse struct {
	Status  string    `json:"status"`
	Message string    `json:"message"`
	Result  []TokenTx `json:"result"`
}

// BalanceResponse — ответ Arbiscan на запрос tokenbalance
type BalanceResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  string `json:"result"`
}

// Hit — найденная транзакция, прошедшая фильтр
type Hit struct {
	Tx     TokenTx
	Amount float64 // в USDC (human-readable)
}
