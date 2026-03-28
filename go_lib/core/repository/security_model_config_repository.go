// Package repository provides the database access layer for security model
// configuration. The configuration is globally unique and used by ShepherdGate
// risk detection.
package repository

import (
	"database/sql"
	"fmt"
	"time"

	"go_lib/core/logging"
)

// SecurityModelConfig is the globally unique security model configuration
// stored with id=1. It defines the LLM settings used by ShepherdGate.
type SecurityModelConfig struct {
	Provider  string `json:"provider"`
	Endpoint  string `json:"endpoint"`
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	SecretKey string `json:"secret_key,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// SecurityModelConfigRepository persists security model configuration.
type SecurityModelConfigRepository struct {
	db *sql.DB
}

// NewSecurityModelConfigRepository creates a repository instance.
func NewSecurityModelConfigRepository(db *sql.DB) *SecurityModelConfigRepository {
	if db == nil {
		db = GetDB()
	}
	return &SecurityModelConfigRepository{db: db}
}

// CreateSecurityModelConfigTable creates the security model configuration table.
// IF NOT EXISTS keeps the operation idempotent.
func CreateSecurityModelConfigTable(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS security_model_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			provider TEXT,
			endpoint TEXT,
			api_key TEXT,
			model TEXT,
			secret_key TEXT,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("failed to create security_model_config table: %w", err)
	}

	logging.Info("Security model config table created/verified successfully")
	return nil
}

// Save persists the globally unique security model configuration.
func (r *SecurityModelConfigRepository) Save(config *SecurityModelConfig) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	config.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	_, err := r.db.Exec(`
		INSERT OR REPLACE INTO security_model_config (id, provider, endpoint, api_key, model, secret_key, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
	`, config.Provider, config.Endpoint, config.APIKey, config.Model, config.SecretKey, config.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save security model config: %w", err)
	}

	logging.Info("Security model config saved: provider=%s, model=%s", config.Provider, config.Model)
	return nil
}

// Get loads the security model configuration.
// It returns nil when the configuration has not been set yet.
func (r *SecurityModelConfigRepository) Get() (*SecurityModelConfig, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	row := r.db.QueryRow(`
		SELECT provider, endpoint, api_key, model, secret_key, updated_at
		FROM security_model_config WHERE id = 1
	`)

	var config SecurityModelConfig
	var provider, endpoint, apiKey, model, secretKey, updatedAt sql.NullString

	err := row.Scan(&provider, &endpoint, &apiKey, &model, &secretKey, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query security model config: %w", err)
	}

	config.Provider = provider.String
	config.Endpoint = endpoint.String
	config.APIKey = apiKey.String
	config.Model = model.String
	config.SecretKey = secretKey.String
	config.UpdatedAt = updatedAt.String

	return &config, nil
}
