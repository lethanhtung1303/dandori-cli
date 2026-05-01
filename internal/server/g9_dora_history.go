// Package server — g9_dora_history.go: /api/g9/dora/history handler.
// Returns last N DORA snapshots as 4 parallel time-series so the dashboard
// can draw sparklines under each DORA tile (deploy_freq / lead_time /
// change_failure / mttr). Empty or single-snapshot DBs flag insufficient.
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

const dorraHistoryDefaultLimit = 12

// handleG9DORAHistory handles GET /api/g9/dora/history.
// Query: ?scope=org|project, ?id=<team>, ?limit=<N> (default 12).
func handleG9DORAHistory(store *db.LocalDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		q := r.URL.Query()
		team := ""
		if q.Get("scope") == "project" {
			team = q.Get("id")
		}
		limit := dorraHistoryDefaultLimit
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}

		snaps, err := store.ListSnapshots(team, "json", limit)
		if err != nil {
			http.Error(w, `{"error":"history query failed"}`, http.StatusInternalServerError)
			return
		}

		if len(snaps) < 2 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"insufficient": true,
				"count":        len(snaps),
			})
			return
		}

		// ListSnapshots is newest-first; reverse so sparklines render
		// left-to-right in chronological order.
		reverseSnapshots(snaps)

		dates := make([]string, 0, len(snaps))
		deploy := make([]float64, 0, len(snaps))
		lead := make([]float64, 0, len(snaps))
		cfr := make([]float64, 0, len(snaps))
		mttr := make([]float64, 0, len(snaps))

		for _, s := range snaps {
			dates = append(dates, s.CreatedAt.UTC().Format(time.RFC3339))
			var raw map[string]any
			if err := json.Unmarshal([]byte(s.Payload), &raw); err != nil {
				deploy = append(deploy, 0)
				lead = append(lead, 0)
				cfr = append(cfr, 0)
				mttr = append(mttr, 0)
				continue
			}
			m := normalizeDoraPayload(raw)
			deploy = append(deploy, doraMetricValue(m["deploy_frequency"]))
			lead = append(lead, doraMetricValue(m["lead_time"]))
			cfr = append(cfr, doraMetricValue(m["change_failure_rate"]))
			mttr = append(mttr, doraMetricValue(m["mttr"]))
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"dates":          dates,
			"deploy_freq":    deploy,
			"lead_time":      lead,
			"change_failure": cfr,
			"mttr":           mttr,
			"count":          len(snaps),
			"insufficient":   false,
		})
	}
}

func reverseSnapshots(s []db.MetricSnapshot) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// doraMetricValue extracts the numeric value from a normalized DORA metric.
// Canonical shape is {value: <num>, unit: ..., rating: ...}; raw payloads may
// instead be a bare number. Returns 0 when neither shape matches.
func doraMetricValue(v any) float64 {
	if m, ok := v.(map[string]any); ok {
		return asFloat(m["value"])
	}
	return asFloat(v)
}

// asFloat coerces a JSON value (number or numeric string) to float64.
// Missing or unparseable values yield 0 — sparklines render a flat gap
// rather than breaking on type errors.
func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case string:
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return f
		}
	}
	return 0
}
