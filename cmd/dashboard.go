package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Open analytics dashboard in browser",
	Long:  "Start a local web server and open the analytics dashboard.",
	RunE:  runDashboard,
}

var dashboardPort int

func init() {
	rootCmd.AddCommand(dashboardCmd)
	dashboardCmd.Flags().IntVarP(&dashboardPort, "port", "p", 8088, "Port to serve dashboard")
}

// newDashboardMux builds and returns the HTTP mux for the dashboard.
// Extracted so tests can call it directly without starting a server.
func newDashboardMux(store *db.LocalDB, jiraBaseURL string) *http.ServeMux {
	mux := http.NewServeMux()

	// Serve dashboard HTML with Jira URL injected
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := strings.ReplaceAll(dashboardHTML, "{{JIRA_BASE_URL}}", jiraBaseURL)
		w.Write([]byte(html)) //nolint:errcheck
	})

	// API endpoints
	mux.HandleFunc("/api/overview", func(w http.ResponseWriter, r *http.Request) {
		runs, cost, tokens, _ := store.GetTotalStats()
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"runs": runs, "cost": cost, "tokens": tokens,
		})
	})

	mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		stats, _ := store.GetAgentStats()
		json.NewEncoder(w).Encode(stats) //nolint:errcheck
	})

	mux.HandleFunc("/api/cost/agent", func(w http.ResponseWriter, r *http.Request) {
		groups, _ := store.GetCostByAgent()
		json.NewEncoder(w).Encode(groups) //nolint:errcheck
	})

	mux.HandleFunc("/api/cost/task", func(w http.ResponseWriter, r *http.Request) {
		groups, _ := store.GetCostByTask()
		json.NewEncoder(w).Encode(groups) //nolint:errcheck
	})

	mux.HandleFunc("/api/cost/day", func(w http.ResponseWriter, r *http.Request) {
		groups, _ := store.GetCostByDay()
		json.NewEncoder(w).Encode(groups) //nolint:errcheck
	})

	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		runs, _ := store.GetRecentRuns(50)
		json.NewEncoder(w).Encode(runs) //nolint:errcheck
	})

	// Quality KPI endpoints
	mux.HandleFunc("/api/quality/regression", qualityHandler(store, "regression"))
	mux.HandleFunc("/api/quality/bugs", qualityHandler(store, "bugs"))
	mux.HandleFunc("/api/quality/cost", qualityHandler(store, "cost"))

	return mux
}

func runDashboard(cmd *cobra.Command, args []string) error {
	store, err := db.Open("")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	// Get Jira URL from config
	jiraBaseURL := "https://jira.example.com"
	if cfg := Config(); cfg != nil && cfg.Jira.BaseURL != "" {
		jiraBaseURL = strings.TrimSuffix(cfg.Jira.BaseURL, "/")
	}

	mux := newDashboardMux(store, jiraBaseURL)

	addr := fmt.Sprintf(":%d", dashboardPort)
	url := fmt.Sprintf("http://localhost:%d", dashboardPort)

	fmt.Printf("Starting dashboard at %s\n", url)
	fmt.Println("Press Ctrl+C to stop")

	// Open browser after short delay
	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(url)
	}()

	return http.ListenAndServe(addr, mux)
}

// qualityHandler returns an HTTP handler for the given quality KPI endpoint.
// kpi must be one of "regression", "bugs", "cost".
func qualityHandler(store *db.LocalDB, kpi string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		by := r.URL.Query().Get("by")
		switch by {
		case "", "agent", "engineer", "sprint":
			// valid
		default:
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"invalid by: must be agent, engineer, or sprint"}`, http.StatusBadRequest)
			return
		}
		since := atoiOr(r.URL.Query().Get("since"), 0)
		w.Header().Set("Content-Type", "application/json")

		var data any
		var qerr error
		switch kpi {
		case "regression":
			data, qerr = store.RegressionRate(by, since)
		case "bugs":
			data, qerr = store.BugRate(by, since)
		case "cost":
			top := atoiOr(r.URL.Query().Get("top"), 50)
			data, qerr = store.QualityAdjustedCost(by, since, top)
		}
		if qerr != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, qerr.Error()), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(data) //nolint:errcheck
	}
}

// atoiOr parses s as an integer; returns def on empty string or parse error.
func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		cmd.Start()
	}
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Dandori Analytics</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-primary: #09090b;
            --bg-secondary: #18181b;
            --bg-tertiary: #27272a;
            --bg-hover: #3f3f46;
            --border: #27272a;
            --border-subtle: #1f1f23;
            --text-primary: #fafafa;
            --text-secondary: #a1a1aa;
            --text-muted: #71717a;
            --accent: #6366f1;
            --accent-hover: #818cf8;
            --success: #22c55e;
            --success-bg: rgba(34, 197, 94, 0.1);
            --warning: #eab308;
            --warning-bg: rgba(234, 179, 8, 0.1);
            --error: #ef4444;
            --error-bg: rgba(239, 68, 68, 0.1);
            --chart-1: #6366f1;
            --chart-2: #22c55e;
            --chart-3: #f59e0b;
            --chart-4: #ef4444;
            --chart-5: #ec4899;
            --chart-6: #8b5cf6;
            --radius: 8px;
            --radius-lg: 12px;
            --shadow: 0 1px 3px rgba(0,0,0,0.4), 0 1px 2px rgba(0,0,0,0.3);
            --shadow-lg: 0 10px 15px -3px rgba(0,0,0,0.4), 0 4px 6px -2px rgba(0,0,0,0.3);
            --transition: all 0.15s ease;
        }

        * { box-sizing: border-box; margin: 0; padding: 0; }

        body {
            font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            line-height: 1.5;
            -webkit-font-smoothing: antialiased;
            -moz-osx-font-smoothing: grayscale;
        }

        /* Scrollbar */
        ::-webkit-scrollbar { width: 8px; height: 8px; }
        ::-webkit-scrollbar-track { background: var(--bg-primary); }
        ::-webkit-scrollbar-thumb { background: var(--bg-tertiary); border-radius: 4px; }
        ::-webkit-scrollbar-thumb:hover { background: var(--bg-hover); }

        /* Layout */
        .app { min-height: 100vh; }

        .sidebar {
            position: fixed;
            left: 0;
            top: 0;
            bottom: 0;
            width: 240px;
            background: var(--bg-secondary);
            border-right: 1px solid var(--border);
            padding: 24px 16px;
            display: flex;
            flex-direction: column;
            z-index: 100;
        }

        .logo {
            display: flex;
            align-items: center;
            gap: 10px;
            padding: 0 8px;
            margin-bottom: 32px;
        }

        .logo-icon {
            width: 32px;
            height: 32px;
            background: linear-gradient(135deg, var(--accent), var(--chart-5));
            border-radius: var(--radius);
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 16px;
        }

        .logo-text {
            font-size: 18px;
            font-weight: 700;
            color: var(--text-primary);
            letter-spacing: -0.5px;
        }

        .nav { flex: 1; }

        .nav-item {
            display: flex;
            align-items: center;
            gap: 10px;
            padding: 10px 12px;
            border-radius: var(--radius);
            color: var(--text-secondary);
            text-decoration: none;
            font-size: 14px;
            font-weight: 500;
            transition: var(--transition);
            cursor: pointer;
            margin-bottom: 4px;
        }

        .nav-item:hover { background: var(--bg-tertiary); color: var(--text-primary); }
        .nav-item.active { background: var(--bg-tertiary); color: var(--text-primary); }

        .nav-icon { width: 18px; height: 18px; opacity: 0.7; }

        .main {
            margin-left: 240px;
            padding: 32px 40px;
            max-width: 1400px;
        }

        /* Header */
        .header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 32px;
        }

        .header-left h1 {
            font-size: 24px;
            font-weight: 700;
            color: var(--text-primary);
            letter-spacing: -0.5px;
        }

        .header-left p {
            font-size: 14px;
            color: var(--text-muted);
            margin-top: 4px;
        }

        .header-actions { display: flex; gap: 12px; align-items: center; }

        .btn {
            display: inline-flex;
            align-items: center;
            gap: 8px;
            padding: 8px 16px;
            border-radius: var(--radius);
            font-size: 14px;
            font-weight: 500;
            border: none;
            cursor: pointer;
            transition: var(--transition);
        }

        .btn-ghost {
            background: transparent;
            color: var(--text-secondary);
            border: 1px solid var(--border);
        }

        .btn-ghost:hover { background: var(--bg-tertiary); color: var(--text-primary); }

        .btn-primary {
            background: var(--accent);
            color: white;
        }

        .btn-primary:hover { background: var(--accent-hover); }

        .btn svg { width: 16px; height: 16px; }

        .last-updated {
            font-size: 12px;
            color: var(--text-muted);
        }

        /* Stats Grid */
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 16px;
            margin-bottom: 24px;
        }

        .stat-card {
            background: var(--bg-secondary);
            border: 1px solid var(--border);
            border-radius: var(--radius-lg);
            padding: 20px 24px;
            transition: var(--transition);
        }

        .stat-card:hover {
            border-color: var(--bg-hover);
            box-shadow: var(--shadow);
        }

        .stat-label {
            font-size: 13px;
            font-weight: 500;
            color: var(--text-muted);
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin-bottom: 8px;
        }

        .stat-value {
            font-size: 32px;
            font-weight: 700;
            color: var(--text-primary);
            letter-spacing: -1px;
            line-height: 1.2;
        }

        .stat-value.accent { color: var(--accent); }
        .stat-value.success { color: var(--success); }
        .stat-value.warning { color: var(--warning); }

        .stat-change {
            display: inline-flex;
            align-items: center;
            gap: 4px;
            font-size: 12px;
            font-weight: 500;
            margin-top: 8px;
            padding: 2px 8px;
            border-radius: 4px;
        }

        .stat-change.up { color: var(--success); background: var(--success-bg); }
        .stat-change.down { color: var(--error); background: var(--error-bg); }

        /* Cards */
        .card {
            background: var(--bg-secondary);
            border: 1px solid var(--border);
            border-radius: var(--radius-lg);
            overflow: hidden;
        }

        .card-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 16px 20px;
            border-bottom: 1px solid var(--border);
        }

        .card-title {
            font-size: 14px;
            font-weight: 600;
            color: var(--text-primary);
        }

        .card-body { padding: 20px; }
        .card-body.no-padding { padding: 0; }

        /* Grid layouts */
        .grid-2 {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 16px;
            margin-bottom: 24px;
        }

        .grid-3-1 {
            display: grid;
            grid-template-columns: 2fr 1fr;
            gap: 16px;
            margin-bottom: 24px;
        }

        /* Charts */
        .chart-container {
            height: 280px;
            position: relative;
        }

        /* Tables */
        .table-wrapper {
            overflow-x: auto;
        }

        table {
            width: 100%;
            border-collapse: collapse;
        }

        th {
            text-align: left;
            padding: 12px 16px;
            font-size: 12px;
            font-weight: 500;
            color: var(--text-muted);
            text-transform: uppercase;
            letter-spacing: 0.5px;
            border-bottom: 1px solid var(--border);
            background: var(--bg-primary);
            position: sticky;
            top: 0;
        }

        td {
            padding: 14px 16px;
            font-size: 14px;
            color: var(--text-secondary);
            border-bottom: 1px solid var(--border-subtle);
        }

        tr { transition: var(--transition); }
        tr:hover { background: var(--bg-tertiary); }
        tr:last-child td { border-bottom: none; }

        /* Status badges */
        .badge {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            padding: 4px 10px;
            border-radius: 9999px;
            font-size: 12px;
            font-weight: 500;
        }

        .badge-success { background: var(--success-bg); color: var(--success); }
        .badge-error { background: var(--error-bg); color: var(--error); }
        .badge-warning { background: var(--warning-bg); color: var(--warning); }

        .badge-dot {
            width: 6px;
            height: 6px;
            border-radius: 50%;
            background: currentColor;
        }

        /* Links */
        .link {
            color: var(--accent);
            text-decoration: none;
            font-weight: 500;
            transition: var(--transition);
        }

        .link:hover { color: var(--accent-hover); text-decoration: underline; }

        /* Task link */
        .task-link {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            color: var(--accent);
            text-decoration: none;
            font-weight: 500;
            font-size: 13px;
            padding: 4px 8px;
            border-radius: var(--radius);
            background: rgba(99, 102, 241, 0.1);
            transition: var(--transition);
        }

        .task-link:hover {
            background: rgba(99, 102, 241, 0.2);
            color: var(--accent-hover);
        }

        .task-link svg {
            width: 12px;
            height: 12px;
            opacity: 0;
            transition: var(--transition);
        }

        .task-link:hover svg { opacity: 1; }

        /* Agent name with icon */
        .agent-cell {
            display: flex;
            align-items: center;
            gap: 10px;
        }

        .agent-avatar {
            width: 28px;
            height: 28px;
            border-radius: var(--radius);
            background: linear-gradient(135deg, var(--chart-1), var(--chart-5));
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 12px;
            font-weight: 600;
            color: white;
        }

        .agent-name { color: var(--text-primary); font-weight: 500; }

        /* Progress bar */
        .progress-bar {
            height: 6px;
            background: var(--bg-tertiary);
            border-radius: 3px;
            overflow: hidden;
            width: 80px;
        }

        .progress-fill {
            height: 100%;
            border-radius: 3px;
            transition: width 0.3s ease;
        }

        .progress-fill.success { background: var(--success); }
        .progress-fill.warning { background: var(--warning); }
        .progress-fill.error { background: var(--error); }

        /* Duration */
        .duration {
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', monospace;
            font-size: 13px;
            color: var(--text-muted);
        }

        /* Cost */
        .cost {
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', monospace;
            font-weight: 500;
        }

        /* Timestamp */
        .timestamp {
            font-size: 13px;
            color: var(--text-muted);
        }

        /* Empty state */
        .empty-state {
            padding: 60px 20px;
            text-align: center;
            color: var(--text-muted);
        }

        .empty-state svg {
            width: 48px;
            height: 48px;
            margin-bottom: 16px;
            opacity: 0.3;
        }

        /* Loading */
        .loading {
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 40px;
        }

        .spinner {
            width: 24px;
            height: 24px;
            border: 2px solid var(--border);
            border-top-color: var(--accent);
            border-radius: 50%;
            animation: spin 0.8s linear infinite;
        }

        @keyframes spin { to { transform: rotate(360deg); } }

        /* Fade in animation */
        @keyframes fadeIn {
            from { opacity: 0; transform: translateY(8px); }
            to { opacity: 1; transform: translateY(0); }
        }

        .fade-in {
            animation: fadeIn 0.3s ease forwards;
        }

        /* Tab buttons */
        .tab-group {
            display: flex;
            gap: 4px;
            padding: 4px;
            background: var(--bg-primary);
            border-radius: var(--radius);
        }

        .tab-btn {
            padding: 6px 12px;
            font-size: 13px;
            font-weight: 500;
            color: var(--text-muted);
            background: transparent;
            border: none;
            border-radius: 6px;
            cursor: pointer;
            transition: var(--transition);
        }

        .tab-btn:hover { color: var(--text-secondary); }
        .tab-btn.active { background: var(--bg-tertiary); color: var(--text-primary); }

        /* Responsive */
        @media (max-width: 1200px) {
            .stats-grid { grid-template-columns: repeat(2, 1fr); }
            .grid-2, .grid-3-1 { grid-template-columns: 1fr; }
        }

        @media (max-width: 768px) {
            .sidebar {
                transform: translateX(-100%);
                transition: transform 0.3s ease;
            }
            .sidebar.open { transform: translateX(0); }
            .main { margin-left: 0; padding: 20px; }
            .stats-grid { grid-template-columns: 1fr; }
            .header { flex-direction: column; align-items: flex-start; gap: 16px; }
        }

        /* Pulse animation for live indicator */
        .live-dot {
            width: 8px;
            height: 8px;
            background: var(--success);
            border-radius: 50%;
            animation: pulse 2s ease-in-out infinite;
        }

        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }

        /* Dimension selector dropdown */
        .dim-selector {
            background: var(--bg-tertiary);
            border: 1px solid var(--border);
            color: var(--text-primary);
            padding: 6px 10px;
            font-size: 13px;
            font-family: inherit;
            border-radius: var(--radius);
            cursor: pointer;
            transition: var(--transition);
        }

        .dim-selector:hover { border-color: var(--bg-hover); }
        .dim-selector:focus { outline: none; border-color: var(--accent); }

        /* Clean badge for quality cost table */
        .clean-badge {
            display: inline-flex;
            align-items: center;
            padding: 2px 8px;
            border-radius: 9999px;
            font-size: 11px;
            font-weight: 500;
        }

        .clean-badge.yes { background: var(--success-bg); color: var(--success); }
        .clean-badge.no  { background: var(--error-bg);   color: var(--error); }
    </style>
</head>
<body>
    <div class="app">
        <aside class="sidebar">
            <div class="logo">
                <div class="logo-icon">D</div>
                <span class="logo-text">Dandori</span>
            </div>
            <nav class="nav">
                <a class="nav-item active" href="#">
                    <svg class="nav-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z"/>
                    </svg>
                    Overview
                </a>
                <a class="nav-item" href="#agents">
                    <svg class="nav-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                    </svg>
                    Agents
                </a>
                <a class="nav-item" href="#runs">
                    <svg class="nav-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/>
                    </svg>
                    Runs
                </a>
                <a class="nav-item" href="#costs">
                    <svg class="nav-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
                    </svg>
                    Costs
                </a>
                <a class="nav-item" href="#quality">
                    <svg class="nav-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
                    </svg>
                    Quality KPI
                </a>
            </nav>
            <div style="padding: 16px 12px; border-top: 1px solid var(--border);">
                <div style="display: flex; align-items: center; gap: 8px;">
                    <div class="live-dot"></div>
                    <span style="font-size: 12px; color: var(--text-muted);">Auto-refresh: 30s</span>
                </div>
            </div>
        </aside>

        <main class="main">
            <header class="header">
                <div class="header-left">
                    <h1>Analytics Dashboard</h1>
                    <p id="last-updated">Last updated: --</p>
                </div>
                <div class="header-actions">
                    <button class="btn btn-ghost" onclick="loadAll()">
                        <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/>
                        </svg>
                        Refresh
                    </button>
                </div>
            </header>

            <!-- Stats Grid -->
            <div class="stats-grid">
                <div class="stat-card fade-in">
                    <div class="stat-label">Total Runs</div>
                    <div class="stat-value" id="total-runs">--</div>
                </div>
                <div class="stat-card fade-in" style="animation-delay: 0.05s;">
                    <div class="stat-label">Total Cost</div>
                    <div class="stat-value success" id="total-cost">--</div>
                </div>
                <div class="stat-card fade-in" style="animation-delay: 0.1s;">
                    <div class="stat-label">Total Tokens</div>
                    <div class="stat-value warning" id="total-tokens">--</div>
                </div>
                <div class="stat-card fade-in" style="animation-delay: 0.15s;">
                    <div class="stat-label">Avg Cost/Run</div>
                    <div class="stat-value accent" id="avg-cost">--</div>
                </div>
            </div>

            <!-- Charts Row -->
            <div class="grid-3-1">
                <div class="card fade-in" style="animation-delay: 0.2s;">
                    <div class="card-header">
                        <span class="card-title">Cost Trend (Last 7 Days)</span>
                        <div class="tab-group">
                            <button class="tab-btn active" data-chart="cost">Cost</button>
                            <button class="tab-btn" data-chart="runs">Runs</button>
                        </div>
                    </div>
                    <div class="card-body">
                        <div class="chart-container">
                            <canvas id="trend-chart"></canvas>
                        </div>
                    </div>
                </div>
                <div class="card fade-in" style="animation-delay: 0.25s;">
                    <div class="card-header">
                        <span class="card-title">Cost by Agent</span>
                    </div>
                    <div class="card-body">
                        <div class="chart-container">
                            <canvas id="cost-chart"></canvas>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Agent Performance -->
            <div class="card fade-in" style="animation-delay: 0.3s; margin-bottom: 24px;" id="agents">
                <div class="card-header">
                    <span class="card-title">Agent Performance</span>
                </div>
                <div class="card-body no-padding">
                    <div class="table-wrapper">
                        <table id="agents-table">
                            <thead>
                                <tr>
                                    <th>Agent</th>
                                    <th>Runs</th>
                                    <th>Success Rate</th>
                                    <th>Total Cost</th>
                                    <th>Avg Cost</th>
                                    <th>Avg Duration</th>
                                    <th>Tokens</th>
                                </tr>
                            </thead>
                            <tbody></tbody>
                        </table>
                    </div>
                </div>
            </div>

            <!-- Quality KPI Section -->
            <div id="quality" style="margin-bottom: 24px;">

            <!-- Quality KPI: Regression Rate -->
            <div class="card fade-in" style="animation-delay: 0.35s; margin-bottom: 16px;">
                <div class="card-header">
                    <span class="card-title">Quality KPI — Regression Rate</span>
                    <select class="dim-selector" data-kpi="regression">
                        <option value="agent">By Agent</option>
                        <option value="engineer">By Engineer</option>
                        <option value="sprint">By Sprint</option>
                    </select>
                </div>
                <div class="card-body no-padding">
                    <div class="table-wrapper" style="max-height: 400px; overflow-y: auto;">
                        <table id="quality-regression-table">
                            <thead>
                                <tr>
                                    <th class="dim-header">Agent</th>
                                    <th>Tasks</th>
                                    <th>Regressed</th>
                                    <th>Regression %</th>
                                </tr>
                            </thead>
                            <tbody></tbody>
                        </table>
                    </div>
                </div>
            </div>

            <!-- Quality KPI: Bug Rate -->
            <div class="card fade-in" style="animation-delay: 0.4s; margin-bottom: 16px;">
                <div class="card-header">
                    <span class="card-title">Quality KPI — Bug Rate</span>
                    <select class="dim-selector" data-kpi="bugs">
                        <option value="agent">By Agent</option>
                        <option value="engineer">By Engineer</option>
                        <option value="sprint">By Sprint</option>
                    </select>
                </div>
                <div class="card-body no-padding">
                    <div class="table-wrapper" style="max-height: 400px; overflow-y: auto;">
                        <table id="quality-bugs-table">
                            <thead>
                                <tr>
                                    <th class="dim-header">Agent</th>
                                    <th>Runs</th>
                                    <th>Bugs</th>
                                    <th>Bugs / Run</th>
                                </tr>
                            </thead>
                            <tbody></tbody>
                        </table>
                    </div>
                </div>
            </div>

            <!-- Quality KPI: Quality-Adjusted Cost -->
            <div class="card fade-in" style="animation-delay: 0.45s;">
                <div class="card-header">
                    <span class="card-title">Quality KPI — Quality-Adjusted Cost</span>
                    <select class="dim-selector" data-kpi="cost">
                        <option value="agent">By Agent</option>
                        <option value="engineer">By Engineer</option>
                        <option value="sprint">By Sprint</option>
                    </select>
                </div>
                <div class="card-body no-padding">
                    <div class="table-wrapper" style="max-height: 400px; overflow-y: auto;">
                        <table id="quality-cost-table">
                            <thead>
                                <tr>
                                    <th>Task</th>
                                    <th class="dim-header">Agent</th>
                                    <th>Cost</th>
                                    <th>Runs</th>
                                    <th>Iterations</th>
                                    <th>Bugs</th>
                                    <th>Clean</th>
                                </tr>
                            </thead>
                            <tbody></tbody>
                        </table>
                    </div>
                </div>
            </div>

            </div> <!-- end #quality -->

            <!-- Recent Runs -->
            <div class="card fade-in" style="animation-delay: 0.5s;" id="runs">
                <div class="card-header">
                    <span class="card-title">Recent Runs</span>
                </div>
                <div class="card-body no-padding">
                    <div class="table-wrapper" style="max-height: 500px; overflow-y: auto;">
                        <table id="runs-table">
                            <thead>
                                <tr>
                                    <th style="width: 100px;">Run ID</th>
                                    <th style="width: 120px;">Task</th>
                                    <th>Agent</th>
                                    <th style="width: 100px;">Status</th>
                                    <th style="width: 100px;">Duration</th>
                                    <th style="width: 100px;">Cost</th>
                                    <th style="width: 100px;">Tokens</th>
                                    <th style="width: 160px;">Started</th>
                                </tr>
                            </thead>
                            <tbody></tbody>
                        </table>
                    </div>
                </div>
            </div>
        </main>
    </div>

    <script>
        // Configuration
        const JIRA_BASE_URL = '{{JIRA_BASE_URL}}'; // Will be replaced with actual Jira URL
        const REFRESH_INTERVAL = 30000;

        // Chart instances
        let costChart = null;
        let trendChart = null;
        let currentTrendMode = 'cost';

        // Color palette
        const chartColors = ['#6366f1', '#22c55e', '#f59e0b', '#ef4444', '#ec4899', '#8b5cf6', '#14b8a6', '#f97316'];

        // Utility functions
        function formatCost(cost) {
            return '$' + (cost || 0).toFixed(2);
        }

        function formatNumber(num) {
            return (num || 0).toLocaleString();
        }

        function formatDuration(seconds) {
            if (seconds < 60) return Math.round(seconds) + 's';
            if (seconds < 3600) return Math.round(seconds / 60) + 'm ' + Math.round(seconds % 60) + 's';
            return Math.round(seconds / 3600) + 'h ' + Math.round((seconds % 3600) / 60) + 'm';
        }

        function formatTime(dateStr) {
            const date = new Date(dateStr);
            const now = new Date();
            const diff = now - date;

            if (diff < 60000) return 'Just now';
            if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
            if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';

            return date.toLocaleDateString('en-US', {
                month: 'short',
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit'
            });
        }

        function getAgentInitials(name) {
            return name.split(/[-_\s]/).map(w => w[0]).join('').toUpperCase().slice(0, 2);
        }

        function getAgentColor(name) {
            let hash = 0;
            for (let i = 0; i < name.length; i++) {
                hash = name.charCodeAt(i) + ((hash << 5) - hash);
            }
            return chartColors[Math.abs(hash) % chartColors.length];
        }

        function createJiraLink(issueKey) {
            if (!issueKey || issueKey === '-') {
                return '<span style="color: var(--text-muted);">-</span>';
            }
            const url = JIRA_BASE_URL ? JIRA_BASE_URL + '/browse/' + issueKey : '#';
            return ` + "`" + `<a href="${url}" target="_blank" rel="noopener" class="task-link">
                ${issueKey}
                <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"/>
                </svg>
            </a>` + "`" + `;
        }

        function createStatusBadge(status) {
            const isDone = status === 'done' || status === 'success' || status === 'completed';
            const badgeClass = isDone ? 'badge-success' : 'badge-error';
            const label = isDone ? 'Done' : status.charAt(0).toUpperCase() + status.slice(1);
            return ` + "`" + `<span class="badge ${badgeClass}"><span class="badge-dot"></span>${label}</span>` + "`" + `;
        }

        function createProgressBar(rate) {
            const fillClass = rate >= 80 ? 'success' : rate >= 50 ? 'warning' : 'error';
            return ` + "`" + `
                <div style="display: flex; align-items: center; gap: 8px;">
                    <div class="progress-bar">
                        <div class="progress-fill ${fillClass}" style="width: ${rate}%"></div>
                    </div>
                    <span style="color: var(--text-${fillClass}); font-weight: 500; font-size: 13px;">${rate.toFixed(1)}%</span>
                </div>
            ` + "`" + `;
        }

        // Load Overview Stats
        async function loadOverview() {
            try {
                const res = await fetch('/api/overview');
                const data = await res.json();

                document.getElementById('total-runs').textContent = formatNumber(data.runs);
                document.getElementById('total-cost').textContent = formatCost(data.cost);
                document.getElementById('total-tokens').textContent = formatNumber(data.tokens);

                const avgCost = data.runs > 0 ? data.cost / data.runs : 0;
                document.getElementById('avg-cost').textContent = formatCost(avgCost);

                document.getElementById('last-updated').textContent = 'Last updated: ' + new Date().toLocaleTimeString();
            } catch (e) {
                console.error('Failed to load overview:', e);
            }
        }

        // Load Agent Stats
        async function loadAgents() {
            try {
                const res = await fetch('/api/agents');
                const data = await res.json();
                const tbody = document.querySelector('#agents-table tbody');

                if (!data || data.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="7" class="empty-state">No agent data available</td></tr>';
                    return;
                }

                tbody.innerHTML = data.map(a => ` + "`" + `
                    <tr>
                        <td>
                            <div class="agent-cell">
                                <div class="agent-avatar" style="background: ${getAgentColor(a.AgentName)}">${getAgentInitials(a.AgentName)}</div>
                                <span class="agent-name">${a.AgentName}</span>
                            </div>
                        </td>
                        <td>${formatNumber(a.RunCount)}</td>
                        <td>${createProgressBar(a.SuccessRate)}</td>
                        <td class="cost">${formatCost(a.TotalCost)}</td>
                        <td class="cost" style="color: var(--text-muted);">${formatCost(a.AvgCost)}</td>
                        <td class="duration">${formatDuration(a.AvgDuration)}</td>
                        <td style="color: var(--text-muted);">${formatNumber(a.TotalTokens)}</td>
                    </tr>
                ` + "`" + `).join('');
            } catch (e) {
                console.error('Failed to load agents:', e);
            }
        }

        // Load Cost Chart (Donut)
        async function loadCostChart() {
            try {
                const res = await fetch('/api/cost/agent');
                const data = await res.json();

                if (costChart) costChart.destroy();

                if (!data || data.length === 0) {
                    document.getElementById('cost-chart').parentElement.innerHTML = '<div class="empty-state">No cost data</div>';
                    return;
                }

                costChart = new Chart(document.getElementById('cost-chart'), {
                    type: 'doughnut',
                    data: {
                        labels: data.map(d => d.Group),
                        datasets: [{
                            data: data.map(d => d.Cost),
                            backgroundColor: chartColors.slice(0, data.length),
                            borderWidth: 0,
                            hoverOffset: 4
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        cutout: '65%',
                        plugins: {
                            legend: {
                                position: 'bottom',
                                labels: {
                                    color: '#a1a1aa',
                                    font: { family: 'Inter', size: 12 },
                                    padding: 16,
                                    usePointStyle: true,
                                    pointStyle: 'circle'
                                }
                            },
                            tooltip: {
                                backgroundColor: '#27272a',
                                titleColor: '#fafafa',
                                bodyColor: '#a1a1aa',
                                borderColor: '#3f3f46',
                                borderWidth: 1,
                                padding: 12,
                                cornerRadius: 8,
                                callbacks: {
                                    label: ctx => ' $' + ctx.raw.toFixed(2)
                                }
                            }
                        }
                    }
                });
            } catch (e) {
                console.error('Failed to load cost chart:', e);
            }
        }

        // Load Trend Chart (Line)
        async function loadTrendChart() {
            try {
                const res = await fetch('/api/cost/day');
                const data = await res.json();

                if (trendChart) trendChart.destroy();

                if (!data || data.length === 0) {
                    document.getElementById('trend-chart').parentElement.innerHTML = '<div class="empty-state">No trend data</div>';
                    return;
                }

                // Sort by date and take last 7 days
                const sortedData = data.sort((a, b) => new Date(a.Group) - new Date(b.Group)).slice(-7);

                const labels = sortedData.map(d => {
                    const date = new Date(d.Group);
                    return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
                });

                const costData = sortedData.map(d => d.Cost);
                const runData = sortedData.map(d => d.RunCount);

                const datasets = currentTrendMode === 'cost' ? [{
                    label: 'Cost',
                    data: costData,
                    borderColor: '#6366f1',
                    backgroundColor: 'rgba(99, 102, 241, 0.1)',
                    fill: true,
                    tension: 0.4,
                    pointRadius: 4,
                    pointHoverRadius: 6,
                    pointBackgroundColor: '#6366f1',
                    pointBorderColor: '#09090b',
                    pointBorderWidth: 2
                }] : [{
                    label: 'Runs',
                    data: runData,
                    borderColor: '#22c55e',
                    backgroundColor: 'rgba(34, 197, 94, 0.1)',
                    fill: true,
                    tension: 0.4,
                    pointRadius: 4,
                    pointHoverRadius: 6,
                    pointBackgroundColor: '#22c55e',
                    pointBorderColor: '#09090b',
                    pointBorderWidth: 2
                }];

                trendChart = new Chart(document.getElementById('trend-chart'), {
                    type: 'line',
                    data: { labels, datasets },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        interaction: {
                            intersect: false,
                            mode: 'index'
                        },
                        scales: {
                            x: {
                                grid: { color: '#27272a', drawBorder: false },
                                ticks: { color: '#71717a', font: { family: 'Inter', size: 11 } }
                            },
                            y: {
                                beginAtZero: true,
                                grid: { color: '#27272a', drawBorder: false },
                                ticks: {
                                    color: '#71717a',
                                    font: { family: 'Inter', size: 11 },
                                    callback: val => currentTrendMode === 'cost' ? '$' + val.toFixed(0) : val
                                }
                            }
                        },
                        plugins: {
                            legend: { display: false },
                            tooltip: {
                                backgroundColor: '#27272a',
                                titleColor: '#fafafa',
                                bodyColor: '#a1a1aa',
                                borderColor: '#3f3f46',
                                borderWidth: 1,
                                padding: 12,
                                cornerRadius: 8,
                                callbacks: {
                                    label: ctx => currentTrendMode === 'cost' ? ' $' + ctx.raw.toFixed(2) : ' ' + ctx.raw + ' runs'
                                }
                            }
                        }
                    }
                });
            } catch (e) {
                console.error('Failed to load trend chart:', e);
            }
        }

        // Load Recent Runs
        async function loadRuns() {
            try {
                const res = await fetch('/api/runs');
                const data = await res.json();
                const tbody = document.querySelector('#runs-table tbody');

                if (!data || data.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="8" class="empty-state">No runs recorded yet</td></tr>';
                    return;
                }

                tbody.innerHTML = data.slice(0, 30).map(r => ` + "`" + `
                    <tr>
                        <td style="font-family: monospace; font-size: 12px; color: var(--text-muted);">${r.ID.substring(0, 8)}</td>
                        <td>${createJiraLink(r.JiraIssueKey)}</td>
                        <td>
                            <div class="agent-cell">
                                <div class="agent-avatar" style="background: ${getAgentColor(r.AgentName)}; width: 24px; height: 24px; font-size: 10px;">${getAgentInitials(r.AgentName)}</div>
                                <span style="color: var(--text-primary);">${r.AgentName}</span>
                            </div>
                        </td>
                        <td>${createStatusBadge(r.Status)}</td>
                        <td class="duration">${formatDuration(r.Duration)}</td>
                        <td class="cost">${formatCost(r.Cost)}</td>
                        <td style="color: var(--text-muted);">${formatNumber(r.Tokens)}</td>
                        <td class="timestamp">${formatTime(r.StartedAt)}</td>
                    </tr>
                ` + "`" + `).join('');
            } catch (e) {
                console.error('Failed to load runs:', e);
            }
        }

        // Tab switching for trend chart
        document.querySelectorAll('.tab-btn').forEach(btn => {
            btn.addEventListener('click', function() {
                document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
                this.classList.add('active');
                currentTrendMode = this.dataset.chart;
                loadTrendChart();
            });
        });

        // Load Quality KPI: Regression Rate
        async function loadQualityRegression(by) {
            by = by || document.querySelector('.dim-selector[data-kpi="regression"]').value;
            try {
                const res = await fetch('/api/quality/regression?by=' + encodeURIComponent(by));
                const rows = await res.json();
                const table = document.querySelector('#quality-regression-table');
                table.querySelector('.dim-header').textContent = by.charAt(0).toUpperCase() + by.slice(1);
                const tbody = table.querySelector('tbody');
                if (!rows || rows.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="4" class="empty-state">No quality KPI data yet</td></tr>';
                    return;
                }
                tbody.innerHTML = rows.map(r => ` + "`" + `<tr>
                    <td style="color: var(--text-primary); font-weight: 500;">${r.group_key || '(unassigned)'}</td>
                    <td>${r.total_tasks}</td>
                    <td>${r.regressed_tasks}</td>
                    <td>${r.regression_pct.toFixed(1)}%</td>
                </tr>` + "`" + `).join('');
            } catch (e) {
                console.error('Failed to load quality regression:', e);
            }
        }

        // Load Quality KPI: Bug Rate
        async function loadQualityBugs(by) {
            by = by || document.querySelector('.dim-selector[data-kpi="bugs"]').value;
            try {
                const res = await fetch('/api/quality/bugs?by=' + encodeURIComponent(by));
                const rows = await res.json();
                const table = document.querySelector('#quality-bugs-table');
                table.querySelector('.dim-header').textContent = by.charAt(0).toUpperCase() + by.slice(1);
                const tbody = table.querySelector('tbody');
                if (!rows || rows.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="4" class="empty-state">No quality KPI data yet</td></tr>';
                    return;
                }
                tbody.innerHTML = rows.map(r => ` + "`" + `<tr>
                    <td style="color: var(--text-primary); font-weight: 500;">${r.group_key || '(unassigned)'}</td>
                    <td>${r.runs}</td>
                    <td>${r.bugs}</td>
                    <td>${r.bugs_per_run.toFixed(2)}</td>
                </tr>` + "`" + `).join('');
            } catch (e) {
                console.error('Failed to load quality bugs:', e);
            }
        }

        // Load Quality KPI: Quality-Adjusted Cost
        async function loadQualityCost(by) {
            by = by || document.querySelector('.dim-selector[data-kpi="cost"]').value;
            try {
                const res = await fetch('/api/quality/cost?by=' + encodeURIComponent(by));
                const rows = await res.json();
                const table = document.querySelector('#quality-cost-table');
                table.querySelector('.dim-header').textContent = by.charAt(0).toUpperCase() + by.slice(1);
                const tbody = table.querySelector('tbody');
                if (!rows || rows.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="7" class="empty-state">No quality KPI data yet</td></tr>';
                    return;
                }
                tbody.innerHTML = rows.map(r => ` + "`" + `<tr>
                    <td style="font-family: monospace; font-size: 12px;">${r.issue_key}</td>
                    <td style="color: var(--text-primary); font-weight: 500;">${r.group_key || '(unassigned)'}</td>
                    <td class="cost">$${r.total_cost_usd.toFixed(4)}</td>
                    <td>${r.run_count}</td>
                    <td>${r.iteration_count}</td>
                    <td>${r.bug_count}</td>
                    <td><span class="clean-badge ${r.is_clean ? 'yes' : 'no'}">${r.is_clean ? 'Yes' : 'No'}</span></td>
                </tr>` + "`" + `).join('');
            } catch (e) {
                console.error('Failed to load quality cost:', e);
            }
        }

        // Wire dropdown change events for Quality KPI
        document.querySelectorAll('.dim-selector').forEach(sel => {
            sel.addEventListener('change', function() {
                const kpi = this.dataset.kpi;
                if (kpi === 'regression') loadQualityRegression(this.value);
                if (kpi === 'bugs')       loadQualityBugs(this.value);
                if (kpi === 'cost')       loadQualityCost(this.value);
            });
        });

        // Load all data
        function loadAll() {
            loadOverview();
            loadAgents();
            loadCostChart();
            loadTrendChart();
            loadRuns();
            loadQualityRegression();
            loadQualityBugs();
            loadQualityCost();
        }

        // Initial load
        loadAll();

        // Auto-refresh
        setInterval(loadAll, REFRESH_INTERVAL);

        // Smooth scroll for nav items
        document.querySelectorAll('.nav-item').forEach(item => {
            item.addEventListener('click', function(e) {
                const href = this.getAttribute('href');
                if (href && href.startsWith('#') && href.length > 1) {
                    e.preventDefault();
                    const target = document.querySelector(href);
                    if (target) {
                        target.scrollIntoView({ behavior: 'smooth', block: 'start' });
                    }
                }
                document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
                this.classList.add('active');
            });
        });
    </script>
</body>
</html>`
