package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
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

func runDashboard(cmd *cobra.Command, args []string) error {
	store, err := db.Open("")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	mux := http.NewServeMux()

	// Serve dashboard HTML
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(dashboardHTML))
	})

	// API endpoints
	mux.HandleFunc("/api/overview", func(w http.ResponseWriter, r *http.Request) {
		runs, cost, tokens, _ := store.GetTotalStats()
		json.NewEncoder(w).Encode(map[string]any{
			"runs": runs, "cost": cost, "tokens": tokens,
		})
	})

	mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		stats, _ := store.GetAgentStats()
		json.NewEncoder(w).Encode(stats)
	})

	mux.HandleFunc("/api/cost/agent", func(w http.ResponseWriter, r *http.Request) {
		groups, _ := store.GetCostByAgent()
		json.NewEncoder(w).Encode(groups)
	})

	mux.HandleFunc("/api/cost/task", func(w http.ResponseWriter, r *http.Request) {
		groups, _ := store.GetCostByTask()
		json.NewEncoder(w).Encode(groups)
	})

	mux.HandleFunc("/api/cost/day", func(w http.ResponseWriter, r *http.Request) {
		groups, _ := store.GetCostByDay()
		json.NewEncoder(w).Encode(groups)
	})

	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		runs, _ := store.GetRecentRuns(50)
		json.NewEncoder(w).Encode(runs)
	})

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
    <title>Dandori Analytics Dashboard</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0f172a; color: #e2e8f0; padding: 20px; }
        .container { max-width: 1400px; margin: 0 auto; }
        h1 { font-size: 24px; margin-bottom: 20px; color: #38bdf8; }
        .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; margin-bottom: 20px; }
        .card { background: #1e293b; border-radius: 12px; padding: 20px; }
        .card h2 { font-size: 14px; color: #94a3b8; margin-bottom: 10px; text-transform: uppercase; letter-spacing: 0.5px; }
        .stat { font-size: 36px; font-weight: bold; color: #f8fafc; }
        .stat.cost { color: #4ade80; }
        .stat.tokens { color: #fbbf24; }
        table { width: 100%; border-collapse: collapse; margin-top: 10px; }
        th, td { text-align: left; padding: 12px; border-bottom: 1px solid #334155; }
        th { color: #94a3b8; font-weight: 500; font-size: 12px; text-transform: uppercase; }
        td { color: #e2e8f0; }
        .success { color: #4ade80; }
        .failed { color: #f87171; }
        .chart-container { height: 300px; }
        .tabs { display: flex; gap: 10px; margin-bottom: 20px; }
        .tab { padding: 8px 16px; background: #334155; border: none; border-radius: 6px; color: #e2e8f0; cursor: pointer; }
        .tab.active { background: #3b82f6; }
        .refresh { float: right; padding: 8px 16px; background: #334155; border: none; border-radius: 6px; color: #e2e8f0; cursor: pointer; }
        .refresh:hover { background: #475569; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🤖 Dandori Analytics <button class="refresh" onclick="loadAll()">↻ Refresh</button></h1>

        <div class="grid">
            <div class="card">
                <h2>Total Runs</h2>
                <div class="stat" id="total-runs">-</div>
            </div>
            <div class="card">
                <h2>Total Cost</h2>
                <div class="stat cost" id="total-cost">-</div>
            </div>
            <div class="card">
                <h2>Total Tokens</h2>
                <div class="stat tokens" id="total-tokens">-</div>
            </div>
        </div>

        <div class="grid">
            <div class="card">
                <h2>Agent Performance</h2>
                <table id="agents-table">
                    <thead><tr><th>Agent</th><th>Runs</th><th>Success</th><th>Cost</th></tr></thead>
                    <tbody></tbody>
                </table>
            </div>
            <div class="card">
                <h2>Cost by Agent</h2>
                <div class="chart-container"><canvas id="cost-chart"></canvas></div>
            </div>
        </div>

        <div class="card">
            <h2>Recent Runs</h2>
            <table id="runs-table">
                <thead><tr><th>ID</th><th>Task</th><th>Agent</th><th>Status</th><th>Duration</th><th>Cost</th><th>Time</th></tr></thead>
                <tbody></tbody>
            </table>
        </div>
    </div>

    <script>
        let costChart = null;

        async function loadOverview() {
            const res = await fetch('/api/overview');
            const data = await res.json();
            document.getElementById('total-runs').textContent = data.runs.toLocaleString();
            document.getElementById('total-cost').textContent = '$' + data.cost.toFixed(2);
            document.getElementById('total-tokens').textContent = data.tokens.toLocaleString();
        }

        async function loadAgents() {
            const res = await fetch('/api/agents');
            const data = await res.json();
            const tbody = document.querySelector('#agents-table tbody');
            tbody.innerHTML = data.map(a => ` + "`" + `
                <tr>
                    <td>${a.AgentName}</td>
                    <td>${a.RunCount}</td>
                    <td class="${a.SuccessRate >= 80 ? 'success' : a.SuccessRate < 50 ? 'failed' : ''}">${a.SuccessRate.toFixed(1)}%</td>
                    <td>$${a.TotalCost.toFixed(2)}</td>
                </tr>
            ` + "`" + `).join('');
        }

        async function loadCostChart() {
            const res = await fetch('/api/cost/agent');
            const data = await res.json();

            if (costChart) costChart.destroy();

            costChart = new Chart(document.getElementById('cost-chart'), {
                type: 'doughnut',
                data: {
                    labels: data.map(d => d.Group),
                    datasets: [{
                        data: data.map(d => d.Cost),
                        backgroundColor: ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899'],
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        legend: { position: 'right', labels: { color: '#e2e8f0' } }
                    }
                }
            });
        }

        async function loadRuns() {
            const res = await fetch('/api/runs');
            const data = await res.json();
            const tbody = document.querySelector('#runs-table tbody');
            tbody.innerHTML = data.slice(0, 20).map(r => ` + "`" + `
                <tr>
                    <td>${r.ID.substring(0, 8)}</td>
                    <td>${r.JiraIssueKey || '-'}</td>
                    <td>${r.AgentName}</td>
                    <td class="${r.Status === 'done' ? 'success' : 'failed'}">${r.Status}</td>
                    <td>${Math.round(r.Duration)}s</td>
                    <td>$${r.Cost.toFixed(2)}</td>
                    <td>${new Date(r.StartedAt).toLocaleString()}</td>
                </tr>
            ` + "`" + `).join('');
        }

        function loadAll() {
            loadOverview();
            loadAgents();
            loadCostChart();
            loadRuns();
        }

        loadAll();
        setInterval(loadAll, 30000); // Auto-refresh every 30s
    </script>
</body>
</html>`
