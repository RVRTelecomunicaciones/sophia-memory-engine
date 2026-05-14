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
	Weights       RankingWeights
	Thresholds    RetrievalThresholds
	ContextBudget ContextBudgetConfig
}

// RankingWeights defines the signals for hybrid retrieval scoring.
//
// The first 7 weighted signals (FTS..ScopeExactness) form the linear-combination
// base score; they should approximately sum to 1.0.
//
// The remaining 3 factors (ADR-0005 P2.3 — sdd_* workload tuning) are NOT
// summed into the base score: they are applied AFTER the base score is
// computed.
//
//   - TopicKeyBoost: multiplicative boost when the query's topic_key matches the
//     record's topic_key exactly. Default 1.5×.
//   - SDDTypeIncrement: additive bump on TypeBoost when the record's type starts
//     with "sdd_" AND that type was explicitly requested in the search filter.
//     This is INCREMENTAL to TypeBoost — it does not replace it. Default 0.10.
//   - TruncatedSnippetPenalty: multiplicative penalty applied to the final score
//     when the snippet contains the ts_headline truncation marker ("..."). The
//     orchestator prefers long-form full-content hits over fragmented ones.
//     Default 0.85×.
//
// These factors are observable through the final score and through the
// (already exposed) TypeBoost component; no new DTO fields are added — public
// API shape is preserved.
type RankingWeights struct {
	FTS            float64
	Trigram        float64
	Recency        float64
	Importance     float64
	TypeBoost      float64
	Freshness      float64
	ScopeExactness float64

	// Post-composition factors (ADR-0005 P2.3).
	TopicKeyBoost           float64
	SDDTypeIncrement        float64
	TruncatedSnippetPenalty float64
}

// RetrievalThresholds defines minimum scores and limits for retrieval.
type RetrievalThresholds struct {
	MinTrigramSimilarity float64
	MinFinalScore        float64
	MaxResults           int
	DefaultResults       int
}

// ContextBudgetConfig defines token budget allocation for context building.
type ContextBudgetConfig struct {
	DefaultMaxTokens    int
	DecisionsPct        float64
	HeuristicsPct       float64
	RecentEpisodicPct   float64
	SemanticPct         float64
	RelatedPct          float64
	TokenEstimateRatio  float64
	MaxGraphExpandDepth int
	MaxGraphExpandCount int
	MaxFanoutPerNode    int // max relations kept per source node after traversal (0 = no limit)
	MaxTotalExpanded    int // hard cap on total expanded nodes returned (0 = default 100)
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
			Weights: RankingWeights{
				FTS:            0.25,
				Trigram:        0.15,
				Recency:        0.12,
				Importance:     0.13,
				TypeBoost:      0.10,
				Freshness:      0.10,
				ScopeExactness: 0.15,

				// ADR-0005 P2.3 — sdd_* workload tuning.
				TopicKeyBoost:           1.5,
				SDDTypeIncrement:        0.10,
				TruncatedSnippetPenalty: 0.85,
			},
			Thresholds: RetrievalThresholds{
				MinTrigramSimilarity: 0.3,
				MinFinalScore:        0.1,
				MaxResults:           100,
				DefaultResults:       20,
			},
			ContextBudget: ContextBudgetConfig{
				DefaultMaxTokens:    4000,
				DecisionsPct:        0.25,
				HeuristicsPct:       0.20,
				RecentEpisodicPct:   0.30,
				SemanticPct:         0.15,
				RelatedPct:          0.10,
				TokenEstimateRatio:  0.25,
				MaxGraphExpandDepth: 1,
				MaxGraphExpandCount: 5,
				MaxFanoutPerNode:    10,
				MaxTotalExpanded:    50,
			},
		},
	}
}
