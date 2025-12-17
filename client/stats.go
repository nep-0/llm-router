package client

import (
	"sync"
	"time"
)

// ModelStats holds performance statistics for a specific model
type ModelStats struct {
	// Total number of requests processed
	TotalRequests int64 `json:"total_requests"`

	// First token time statistics (milliseconds)
	FirstTokenTimeAvg float64 `json:"first_token_time_avg_ms"`
	FirstTokenTimeMin float64 `json:"first_token_time_min_ms"`
	FirstTokenTimeMax float64 `json:"first_token_time_max_ms"`

	// Tokens per second statistics
	TokensPerSecondAvg float64 `json:"tokens_per_second_avg"`
	TokensPerSecondMin float64 `json:"tokens_per_second_min"`
	TokensPerSecondMax float64 `json:"tokens_per_second_max"`

	// Total tokens processed
	TotalTokens int64 `json:"total_tokens"`

	// Internal tracking (not exported)
	firstTokenTimeSum float64
	tokensPerSecSum   float64
}

// PerformanceStats tracks performance metrics for all models
type PerformanceStats struct {
	mu    sync.RWMutex
	stats map[string]*ModelStats // keyed by "provider:model"
}

// NewPerformanceStats creates a new PerformanceStats instance
func NewPerformanceStats() *PerformanceStats {
	return &PerformanceStats{
		stats: make(map[string]*ModelStats),
	}
}

// RecordRequest records performance metrics for a completed request
func (ps *PerformanceStats) RecordRequest(provider, model string, firstTokenTime time.Duration, totalTokens int64, totalDuration time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	key := provider + ":" + model
	s, exists := ps.stats[key]
	if !exists {
		s = &ModelStats{
			FirstTokenTimeMin:  -1,
			TokensPerSecondMin: -1,
		}
		ps.stats[key] = s
	}

	s.TotalRequests++
	s.TotalTokens += totalTokens

	// Record first token time (in milliseconds)
	if firstTokenTime > 0 {
		ftMs := float64(firstTokenTime.Milliseconds())
		s.firstTokenTimeSum += ftMs
		s.FirstTokenTimeAvg = s.firstTokenTimeSum / float64(s.TotalRequests)

		if s.FirstTokenTimeMin < 0 || ftMs < s.FirstTokenTimeMin {
			s.FirstTokenTimeMin = ftMs
		}
		if ftMs > s.FirstTokenTimeMax {
			s.FirstTokenTimeMax = ftMs
		}
	}

	// Calculate and record tokens per second
	if totalDuration > 0 && totalTokens > 0 {
		tps := float64(totalTokens) / totalDuration.Seconds()
		s.tokensPerSecSum += tps
		s.TokensPerSecondAvg = s.tokensPerSecSum / float64(s.TotalRequests)

		if s.TokensPerSecondMin < 0 || tps < s.TokensPerSecondMin {
			s.TokensPerSecondMin = tps
		}
		if tps > s.TokensPerSecondMax {
			s.TokensPerSecondMax = tps
		}
	}
}

// GetStats returns a copy of the stats for all models
func (ps *PerformanceStats) GetStats() map[string]ModelStats {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make(map[string]ModelStats)
	for k, v := range ps.stats {
		// Return a copy with cleaned up values
		stat := *v
		if stat.FirstTokenTimeMin < 0 {
			stat.FirstTokenTimeMin = 0
		}
		if stat.TokensPerSecondMin < 0 {
			stat.TokensPerSecondMin = 0
		}
		// Clear internal fields
		stat.firstTokenTimeSum = 0
		stat.tokensPerSecSum = 0
		result[k] = stat
	}
	return result
}

// GetModelStats returns the stats for a specific model
func (ps *PerformanceStats) GetModelStats(provider, model string) *ModelStats {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	key := provider + ":" + model
	if s, exists := ps.stats[key]; exists {
		stat := *s
		if stat.FirstTokenTimeMin < 0 {
			stat.FirstTokenTimeMin = 0
		}
		if stat.TokensPerSecondMin < 0 {
			stat.TokensPerSecondMin = 0
		}
		stat.firstTokenTimeSum = 0
		stat.tokensPerSecSum = 0
		return &stat
	}
	return nil
}

// Global performance stats instance
var GlobalStats = NewPerformanceStats()
