package usagestats

import "time"

type RepositoryStat struct {
	Repository  string  `json:"repository"`
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`
	ActualCost  float64 `json:"actual_cost"`
}

type RepositoryStatsPage struct {
	Repositories []RepositoryStat `json:"repositories"`
	Page         int              `json:"page"`
	PageSize     int              `json:"page_size"`
	HasMore      bool             `json:"has_more"`
	GeneratedAt  time.Time        `json:"generated_at"`
}
