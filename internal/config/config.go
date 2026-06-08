package config

import "github.com/caarlos0/env/v11"

// DocumentIntelligence contém as configurações necessárias para o client do pacote docintel.
type DocumentIntelligence struct {
	Key      string `env:"AZURE_DOCINTEL_KEY"`
	Endpoint string `env:"AZURE_DOCINTEL_ENDPOINT"`
}

// Config contém as configurações da aplicação.
type Config struct {
	PostgresURL string `env:"POSTGRES_URL,notEmpty"`

	DocIntel DocumentIntelligence
}

// NewFromEnv cria uma nova [Config] com base nas variáveis de ambiente definidas no sistema operacional.
func NewFromEnv() (*Config, error) {
	var cfg Config
	err := env.Parse(&cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
