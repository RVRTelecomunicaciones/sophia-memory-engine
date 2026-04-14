package config

import "time"

// AppConfig holds the complete application configuration.
type AppConfig struct {
	Server    ServerConfig
	Database  DatabaseConfig
	Retrieval RetrievalConfig
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// RetrievalConfig holds retrieval subsystem settings.
type RetrievalConfig struct {
	Ranking       RankingWeights
	Thresholds    RetrievalThresholds
	ContextBudget ContextBudgetConfig
}

// RankingWeights defines weights for hybrid retrieval scoring.
type RankingWeights struct {
	Recency    float64
	Relevance  float64
	Importance float64
	Freshness  float64
}

// RetrievalThresholds defines minimum scores for retrieval results.
type RetrievalThresholds struct {
	MinRelevanceScore float64
	MinFinalScore     float64
	MaxResults        int
}

// ContextBudgetConfig defines limits for context building.
type ContextBudgetConfig struct {
	MaxTokens          int
	MaxMemories        int
	MaxDecisions       int
	MaxHeuristics      int
	MaxRelationDepth   int
	IncludeEvidence    bool
	IncludeProvenance  bool
}

// DefaultConfig returns the default application configuration.
func DefaultConfig() AppConfig {
	return AppConfig{
		Server: ServerConfig{
			Port:            8080,
			ReadTimeout:     15 * time.Second,
			WriteTimeout:    15 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Database: DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			User:            "memory_engine",
			Password:        "",
			DBName:          "memory_engine",
			SSLMode:         "disable",
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
			ConnMaxIdleTime: 1 * time.Minute,
		},
		Retrieval: RetrievalConfig{
			Ranking: RankingWeights{
				Recency:    0.25,
				Relevance:  0.35,
				Importance: 0.25,
				Freshness:  0.15,
			},
			Thresholds: RetrievalThresholds{
				MinRelevanceScore: 0.3,
				MinFinalScore:     0.4,
				MaxResults:        50,
			},
			ContextBudget: ContextBudgetConfig{
				MaxTokens:         4000,
				MaxMemories:       20,
				MaxDecisions:      10,
				MaxHeuristics:     10,
				MaxRelationDepth:  3,
				IncludeEvidence:   true,
				IncludeProvenance: true,
			},
		},
	}
}
