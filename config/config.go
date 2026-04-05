package config

import "time"

type Config struct {
	APIKey         string
	BaseURL        string
	BridgeAddress  string
	USDCAddress    string
	MinUSD         float64
	MaxUSD         float64
	PollInterval   time.Duration
	ResearchPeriod time.Duration
	DBPath         string
}

func Default() *Config {
	return &Config{
		APIKey:         "IN1VVGWADFKQV62W2XJ74F6FZHQ9F4UXXW",
		BaseURL:        "https://api.etherscan.io/v2/api",
		BridgeAddress:  "0x2Df1c51E09aECF9cacB7bc98cB1742757f163dF7",
		USDCAddress:    "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
		MinUSD:         8_000,
		MaxUSD:         60_000,
		PollInterval:   30 * time.Second,
		ResearchPeriod: 48 * time.Hour,
		DBPath:         "wallets.db",
	}
}
