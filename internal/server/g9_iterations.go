// Package server — g9_iterations.go: /api/g9/iterations handler.
// Returns a histogram of tasks bucketed by their iteration round count
// (true rework signal: 1, 2, 3, 4, 5+ rounds), scoped by role/period.
package server

import (
	"encoding/json"
	"net/http"

	"github.com/phuc-nt/dandori-cli/internal/db"
)

// iterationRoundHistogram queries iteration round counts per task in the
// given window, optionally scoped to a project, and returns the histogram.
func iterationRoundHistogram(store *db.LocalDB, role, id string, w *Window) (map[string]any, error) {
	projectKey := ""
	if role == "project" && id != "" {
		projectKey = id
	}
	buckets, total, err := store.IterationDistribution(w.Start, w.End, projectKey)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, len(buckets))
	for i, b := range buckets {
		out[i] = map[string]any{
			"label": b.Label,
			"count": b.Count,
		}
	}
	return map[string]any{
		"buckets": out,
		"total":   total,
	}, nil
}

// handleG9Iterations handles GET /api/g9/iterations.
// Supports ?role=, ?id=, ?period=, ?from=, ?to= params.
func handleG9Iterations(store *db.LocalDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		q := r.URL.Query()
		role := q.Get("role")
		if role == "" {
			role = "org"
		}
		id := q.Get("id")

		cur, _, err := ParsePeriodWindow(r, role)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		result, err := iterationRoundHistogram(store, role, id, cur)
		if err != nil {
			http.Error(w, `{"error":"iterations query failed"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(result) //nolint:errcheck
	}
}
