// Package server — g9_mix_leaderboard.go: /api/g9/mix-leaderboard handler.
// Surfaces engineer × agent leaderboard (runs, cost, avg cost) on org view.
// Pure surfacing — backed by db.GetMixLeaderboard which `analytics all` already
// uses, so dashboard and CLI agree on numbers.
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

const mixLeaderboardDefaultLimit = 20
const mixLeaderboardDefaultDays = 28

// handleG9MixLeaderboard handles GET /api/g9/mix-leaderboard.
// Query: ?period=<days> (default 28), ?limit=<N> (default 20).
// Returns {rows:[{engineer,agent,run_count,total_cost,avg_cost}], period_days, limit}.
func handleG9MixLeaderboard(store *db.LocalDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		days := mixLeaderboardDefaultDays
		if v := r.URL.Query().Get("period"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				days = n
			}
		}
		limit := mixLeaderboardDefaultLimit
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}

		rows, err := store.GetMixLeaderboard(days)
		if err != nil {
			http.Error(w, `{"error":"leaderboard query failed"}`, http.StatusInternalServerError)
			return
		}

		// GetMixLeaderboard is sorted by run_count DESC, total_cost DESC.
		// Trim to limit; null slice → [] for stable JSON shape.
		if len(rows) > limit {
			rows = rows[:limit]
		}
		if rows == nil {
			rows = []db.MixRow{}
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"rows":        rows,
			"period_days": days,
			"limit":       limit,
		})
	}
}
