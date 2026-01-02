package management

import (
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) GetUsageStatistics(c *gin.Context) {
	if h == nil || h.usagePlugin == nil {
		respondOK(c, UsageStatsResponse{})
		return
	}

	counters := h.usagePlugin.GetCounters()

	response := UsageStatsResponse{
		Counters: UsageCounters{
			TotalRequests: counters.TotalRequests,
			SuccessCount:  counters.SuccessCount,
			FailureCount:  counters.FailureCount,
			TotalTokens:   counters.TotalTokens,
		},
	}

	backend := h.usagePlugin.GetBackend()
	if backend != nil {
		ctx := c.Request.Context()
		cutoff := time.Now().AddDate(0, 0, -30) // Last 30 days

		if dailyStats, err := backend.QueryDailyStats(ctx, cutoff); err == nil && len(dailyStats) > 0 {
			byDay := make([]UsageDayStats, 0, len(dailyStats))
			for _, d := range dailyStats {
				byDay = append(byDay, UsageDayStats{
					Day:      d.Day,
					Requests: d.Requests,
					Tokens:   d.Tokens,
				})
			}
			response.ByDay = byDay
		}

		if hourlyStats, err := backend.QueryHourlyStats(ctx, cutoff); err == nil && len(hourlyStats) > 0 {
			byHour := make([]UsageHourStats, 0, len(hourlyStats))
			for _, h := range hourlyStats {
				byHour = append(byHour, UsageHourStats{
					Hour:     h.Hour,
					Requests: h.Requests,
					Tokens:   h.Tokens,
				})
			}
			response.ByHour = byHour
		}

		if apiStats, err := backend.QueryAPIStats(ctx, cutoff); err == nil && len(apiStats) > 0 {
			byAPI := make(map[string]UsageAPIStats)
			for _, a := range apiStats {
				api, ok := byAPI[a.APIKey]
				if !ok {
					api = UsageAPIStats{
						Models: make(map[string]UsageModelStats),
					}
				}
				api.TotalRequests += a.Requests
				api.TotalTokens += a.Tokens
				api.Models[a.Model] = UsageModelStats{
					TotalRequests: a.Requests,
					TotalTokens:   a.Tokens,
				}
				byAPI[a.APIKey] = api
			}
			response.ByAPI = byAPI
		}
	}

	respondOK(c, response)
}
