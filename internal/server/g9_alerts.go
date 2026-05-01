// Package server — g9_alerts.go: /api/g9/alerts handler.
// Surfaces threshold breach alerts (cost_multiple, ac_dip) computed by
// analytics.DetectAlerts, the same source of truth as `dandori analytics all`.
package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/analytics"
	"github.com/phuc-nt/dandori-cli/internal/db"
)

// handleG9Alerts handles GET /api/g9/alerts.
// Query: ?since=<days> (default 7).
// Returns {"alerts":[{kind,severity,message,drilldown_url}]}.
func handleG9Alerts(store *db.LocalDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Default 30-day window matches `dandori analytics all` so dashboard
		// banner counts stay aligned with the CLI's alerts section.
		days := 30
		if v := r.URL.Query().Get("since"); v != "" {
			if n, err := time.ParseDuration(v + "h"); err == nil && n > 0 {
				days = int(n.Hours() / 24)
			}
		}

		win := analytics.Window{Since: time.Duration(days) * 24 * time.Hour}
		snap, err := analytics.BuildSnapshot(store, win, analytics.DefaultThresholds())
		if err != nil {
			http.Error(w, `{"error":"alerts query failed"}`, http.StatusInternalServerError)
			return
		}

		alerts := snap.Alerts
		if alerts == nil {
			alerts = []analytics.Alert{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"alerts": alerts})
	}
}
