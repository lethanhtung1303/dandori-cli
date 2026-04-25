#!/bin/zsh
# Enterprise Simulation E2E Test
# Simulates: diverse agents, teams, projects over 30+ days
# Goal: Answer 5 business questions via dashboard

set -e

DANDORI="./bin/dandori"
DB_PATH="$HOME/.dandori/local.db"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
echo "${BLUE}║     Enterprise Simulation: Multi-Team Scale            ║${NC}"
echo "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"

# ============================================================
# SETUP
# ============================================================
echo "\n${BLUE}========== SETUP ==========${NC}"
echo "Clearing old test data..."
rm -f "$DB_PATH"
$DANDORI init --no-shell 2>/dev/null || true

# Create test workspace
TEST_WS="/tmp/dandori-enterprise-sim"
rm -rf "$TEST_WS"
mkdir -p "$TEST_WS"
cd "$TEST_WS"
git init -q
git config user.email "test@dandori.dev"
git config user.name "Dandori Test"
echo "# Enterprise Simulation" > README.md
git add . && git commit -q -m "init"

# Get workstation ID
WORKSTATION_ID=$(sqlite3 "$DB_PATH" "SELECT value FROM kv WHERE key='workstation_id'" 2>/dev/null || echo "ws-sim-001")

# ============================================================
# DEFINE: Teams, Agents, Projects
# ============================================================
typeset -A AGENT_QUALITY
AGENT_QUALITY=(
    alice 0.92  bob 0.75  charlie 0.58
    david 0.88  eve 0.70  frank 0.45
    grace 0.95  henry 0.80  ivan 0.62
    julia 0.90  kevin 0.72  lisa 0.55
    mike 0.93  nancy 0.78  oscar 0.38
)

typeset -A AGENT_TEAM
AGENT_TEAM=(
    alice platform  bob platform  charlie platform
    david payments  eve payments  frank payments
    grace mobile    henry mobile  ivan mobile
    julia infra     kevin infra   lisa infra
    mike security   nancy security  oscar security
)

typeset -A TEAM_PROJECT
TEAM_PROJECT=(
    platform PLAT
    payments PAY
    mobile MOB
    infra INFRA
    security SEC
)

PROJECTS=(PLAT PAY MOB INFRA SEC)
TASK_TYPES=(Bug Story Task Security Migration Refactor)
ALL_AGENTS=(alice bob charlie david eve frank grace henry ivan julia kevin lisa mike nancy oscar)

echo "Teams: platform payments mobile infra security"
echo "Agents: ${ALL_AGENTS[@]}"

# ============================================================
# FUNCTION: Insert run
# ============================================================
insert_run() {
    local agent=$1
    local project=$2
    local task_type=$3
    local quality_score=$4
    local cost=$5
    local tokens_in=$6
    local tokens_out=$7
    local duration=$8
    local days_ago=$9
    local exit_code=${10:-0}

    local run_id=$(uuidgen | tr '[:upper:]' '[:lower:]')
    local task_num=$((RANDOM % 500 + 1))
    local jira_key="${project}-${task_num}"
    local git_before=$(openssl rand -hex 20 | head -c 7)
    local git_after=$(openssl rand -hex 20 | head -c 7)
    local created_at=$(date -v-${days_ago}d "+%Y-%m-%dT%H:%M:%SZ")
    local started_at=$created_at

    local model="claude-sonnet-4-6"
    if (( $(echo "$cost > 1.0" | bc -l) )); then
        model="claude-opus-4-6"
    elif (( $(echo "$cost < 0.1" | bc -l) )); then
        model="claude-haiku-4-5"
    fi

    local run_status="completed"
    [[ $exit_code -ne 0 ]] && run_status="failed"

    sqlite3 "$DB_PATH" "INSERT INTO runs (
        id, agent_name, agent_type, user, workstation_id, jira_issue_key, cwd,
        git_head_before, git_head_after, exit_code, duration_sec,
        input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
        model, cost_usd, synced, status, started_at, created_at
    ) VALUES (
        '$run_id', '$agent', 'claude_code', '$agent@company.com', '$WORKSTATION_ID', '$jira_key', '$TEST_WS',
        '$git_before', '$git_after', $exit_code, $duration,
        $tokens_in, $tokens_out, 0, 0,
        '$model', $cost, 1, '$run_status', '$started_at', '$created_at'
    );"

    # Quality metrics
    local lint_before=$((RANDOM % 20 + 5))
    local lint_delta=0
    if (( $(echo "$quality_score > 0.8" | bc -l) )); then
        lint_delta=$(( -1 * (RANDOM % 8 + 2) ))
    elif (( $(echo "$quality_score < 0.5" | bc -l) )); then
        lint_delta=$((RANDOM % 5 + 1))
    else
        lint_delta=$(( (RANDOM % 6) - 3 ))
    fi
    local lint_after=$((lint_before + lint_delta))
    [[ $lint_after -lt 0 ]] && lint_after=0

    local tests_before=$((50 + RANDOM % 100))
    local tests_delta=0
    if (( $(echo "$quality_score > 0.8" | bc -l) )); then
        tests_delta=$((RANDOM % 15 + 5))
    elif (( $(echo "$quality_score < 0.5" | bc -l) )); then
        tests_delta=$(( -1 * (RANDOM % 5) ))
    else
        tests_delta=$(( (RANDOM % 10) - 2 ))
    fi
    local tests_after=$((tests_before + tests_delta))

    local lines_added=$((RANDOM % 500 + 50))
    local lines_removed=$((RANDOM % 200 + 20))
    local files_changed=$((RANDOM % 15 + 1))
    local commit_count=$((RANDOM % 5 + 1))

    sqlite3 "$DB_PATH" "INSERT INTO quality_metrics (
        run_id, lint_errors_before, lint_errors_after, lint_warnings_before, lint_warnings_after,
        tests_total_before, tests_passed_before, tests_failed_before,
        tests_total_after, tests_passed_after, tests_failed_after,
        lines_added, lines_removed, files_changed, commit_count, commit_msg_quality
    ) VALUES (
        '$run_id', $lint_before, $lint_after, 0, 0,
        $tests_before, $tests_before, 0,
        $tests_after, $tests_after, 0,
        $lines_added, $lines_removed, $files_changed, $commit_count, $quality_score
    );"
}

# ============================================================
# GENERATE: 30-day history
# ============================================================
echo "\n${BLUE}========== GENERATING 30-DAY HISTORY ==========${NC}"

for days_ago in {0..30}; do
    day_of_week=$((days_ago % 7))
    if [[ $day_of_week == 0 || $day_of_week == 6 ]]; then
        runs_today=$((RANDOM % 12 + 6))
    else
        runs_today=$((RANDOM % 35 + 20))
    fi

    echo -n "${YELLOW}Day -$days_ago:${NC} "

    for i in {1..$runs_today}; do
        agent=${ALL_AGENTS[$((RANDOM % ${#ALL_AGENTS[@]} + 1))]}
        team=${AGENT_TEAM[$agent]:-platform}
        project=${TEAM_PROJECT[$team]:-PLAT}

        # 20% cross-team work
        [[ $((RANDOM % 5)) == 0 ]] && project=${PROJECTS[$((RANDOM % ${#PROJECTS[@]} + 1))]}

        # Task type distribution
        task_rand=$((RANDOM % 100))
        if [[ $task_rand -lt 30 ]]; then task_type="Bug"
        elif [[ $task_rand -lt 55 ]]; then task_type="Story"
        elif [[ $task_rand -lt 75 ]]; then task_type="Task"
        elif [[ $task_rand -lt 85 ]]; then task_type="Refactor"
        elif [[ $task_rand -lt 95 ]]; then task_type="Security"
        else task_type="Migration"
        fi

        quality=${AGENT_QUALITY[$agent]:-0.7}

        case $task_type in
            Bug)       base_cost="0.12" ;;
            Story)     base_cost="0.42" ;;
            Task)      base_cost="0.22" ;;
            Security)  base_cost="0.52" ;;
            Migration) base_cost="0.95" ;;
            Refactor)  base_cost="0.32" ;;
        esac

        variance=$(echo "scale=2; ($RANDOM % 50) / 100" | bc)
        cost=$(echo "scale=2; $base_cost + $variance" | bc)
        tokens_in=$((RANDOM % 6000 + 1500))
        tokens_out=$((RANDOM % 2500 + 800))
        duration=$((RANDOM % 400 + 45))

        exit_code=0
        quality_int=$(printf "%.0f" $(echo "$quality * 100" | bc))
        fail_threshold=$((100 - quality_int))
        [[ $((RANDOM % 100)) -lt $fail_threshold ]] && exit_code=1

        insert_run "$agent" "$project" "$task_type" "$quality" "$cost" "$tokens_in" "$tokens_out" "$duration" "$days_ago" "$exit_code"
    done

    echo "$runs_today runs"
done

# ============================================================
# SPECIAL SCENARIOS
# ============================================================
echo "\n${BLUE}========== SPECIAL SCENARIOS ==========${NC}"

echo "• Oscar (low quality) security violations..."
for d in 2 5 8 12; do
    insert_run "oscar" "SEC" "Security" "0.25" "0.78" "4200" "1800" "280" "$d" "0"
done

echo "• Frank's failed migrations..."
insert_run "frank" "PAY" "Migration" "0.30" "1.45" "9500" "4200" "520" "1" "1"
insert_run "frank" "PAY" "Migration" "0.28" "1.32" "8800" "3900" "480" "3" "1"

echo "• Alice (senior dev) 6-month history..."
for days in {35..180..3}; do
    task_types=(Story Task Refactor Security)
    task=${task_types[$((RANDOM % 4 + 1))]}
    cost=$(echo "scale=2; 0.45 + ($RANDOM % 40) / 100" | bc)
    insert_run "alice" "PLAT" "$task" "0.94" "$cost" "$((RANDOM % 5000 + 2500))" "$((RANDOM % 2000 + 1000))" "$((RANDOM % 350 + 120))" "$days" "0"
done

echo "• High-cost opus runs..."
for agent in grace julia mike; do
    for d in 1 7 14 21; do
        insert_run "$agent" "${TEAM_PROJECT[${AGENT_TEAM[$agent]}]}" "Story" "${AGENT_QUALITY[$agent]}" "2.35" "15000" "8000" "900" "$d" "0"
    done
done

echo "• Team comparison data..."
for d in {1..14}; do
    insert_run "david" "PAY" "Story" "0.88" "0.55" "4500" "2000" "280" "$d" "0"
    insert_run "eve" "PAY" "Bug" "0.70" "0.35" "2800" "1200" "180" "$d" "0"
    insert_run "frank" "PAY" "Task" "0.45" "0.42" "3200" "1400" "$((RANDOM % 200 + 150))" "$d" "$((RANDOM % 3 == 0 ? 1 : 0))"
    insert_run "alice" "PLAT" "Story" "0.92" "0.62" "4800" "2100" "260" "$d" "0"
    insert_run "bob" "PLAT" "Bug" "0.75" "0.38" "3000" "1300" "190" "$d" "0"
    insert_run "charlie" "PLAT" "Task" "0.58" "0.45" "3400" "1500" "$((RANDOM % 200 + 160))" "$d" "$((RANDOM % 4 == 0 ? 1 : 0))"
done

# ============================================================
# SUMMARY
# ============================================================
echo "\n${BLUE}========== SIMULATION COMPLETE ==========${NC}"

total_runs=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM runs")
total_cost=$(sqlite3 "$DB_PATH" "SELECT ROUND(SUM(cost_usd), 2) FROM runs")
unique_agents=$(sqlite3 "$DB_PATH" "SELECT COUNT(DISTINCT agent_name) FROM runs")
unique_tasks=$(sqlite3 "$DB_PATH" "SELECT COUNT(DISTINCT jira_issue_key) FROM runs")
avg_quality=$(sqlite3 "$DB_PATH" "SELECT ROUND(AVG(commit_msg_quality), 2) FROM quality_metrics")

echo "Total runs:      ${GREEN}$total_runs${NC}"
echo "Total cost:      ${GREEN}\$$total_cost${NC}"
echo "Unique agents:   ${GREEN}$unique_agents${NC}"
echo "Unique tasks:    ${GREEN}$unique_tasks${NC}"
echo "Avg quality:     ${GREEN}$avg_quality${NC}"

# ============================================================
# BUSINESS QUESTIONS
# ============================================================
echo "\n${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
echo "${BLUE}║           ANSWERING 5 BUSINESS QUESTIONS               ║${NC}"
echo "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"

echo "\n${YELLOW}Q1: Tháng này tốn bao nhiêu tiền API, chia theo project?${NC}"
sqlite3 -header -column "$DB_PATH" "
SELECT
    SUBSTR(jira_issue_key, 1, INSTR(jira_issue_key, '-')-1) as PROJECT,
    COUNT(*) as RUNS,
    ROUND(SUM(cost_usd), 2) as TOTAL_COST,
    ROUND(AVG(cost_usd), 2) as AVG_COST
FROM runs
WHERE created_at >= date('now', '-30 days')
GROUP BY PROJECT
ORDER BY TOTAL_COST DESC
"

echo "\n${YELLOW}Q2: Agent viết code kém (security issues) - ai chịu trách nhiệm?${NC}"
sqlite3 -header -column "$DB_PATH" "
SELECT
    r.agent_name as AGENT,
    COUNT(*) as LOW_QUALITY_RUNS,
    ROUND(AVG(qm.commit_msg_quality) * 100, 0) || '%' as AVG_QUALITY,
    SUM(CASE WHEN qm.lint_errors_after > qm.lint_errors_before THEN 1 ELSE 0 END) as LINT_REGRESSIONS
FROM runs r
JOIN quality_metrics qm ON r.id = qm.run_id
WHERE qm.commit_msg_quality < 0.5
GROUP BY r.agent_name
ORDER BY LOW_QUALITY_RUNS DESC
LIMIT 5
"

echo "\n${YELLOW}Q3: Migration làm sập staging - ai approve?${NC}"
sqlite3 -header -column "$DB_PATH" "
SELECT
    agent_name as AGENT,
    jira_issue_key as TASK,
    status as STATUS,
    ROUND(cost_usd, 2) as COST,
    substr(created_at, 1, 16) as CREATED
FROM runs
WHERE status = 'failed'
ORDER BY created_at DESC
LIMIT 10
"

echo "\n${YELLOW}Q4: Team A (platform) vs Team B (payments) - ai viết code tốt hơn?${NC}"
sqlite3 -header -column "$DB_PATH" "
SELECT
    CASE
        WHEN r.agent_name IN ('alice', 'bob', 'charlie') THEN 'platform'
        WHEN r.agent_name IN ('david', 'eve', 'frank') THEN 'payments'
        WHEN r.agent_name IN ('grace', 'henry', 'ivan') THEN 'mobile'
        WHEN r.agent_name IN ('julia', 'kevin', 'lisa') THEN 'infra'
        ELSE 'security'
    END as TEAM,
    COUNT(*) as RUNS,
    ROUND(AVG(qm.commit_msg_quality) * 100, 0) || '%' as QUALITY,
    ROUND(AVG(qm.lint_errors_after - qm.lint_errors_before), 1) as LINT_DELTA,
    ROUND(AVG(qm.tests_passed_after - qm.tests_passed_before), 1) as TESTS_DELTA,
    ROUND(100.0 * SUM(CASE WHEN r.status = 'completed' THEN 1 ELSE 0 END) / COUNT(*), 0) || '%' as SUCCESS_RATE
FROM runs r
JOIN quality_metrics qm ON r.id = qm.run_id
GROUP BY TEAM
ORDER BY QUALITY DESC
"

echo "\n${YELLOW}Q5: Senior dev (alice) 6 tháng kinh nghiệm - ở đâu?${NC}"
sqlite3 -header -column "$DB_PATH" "
SELECT
    agent_name as AGENT,
    COUNT(*) as TOTAL_RUNS,
    ROUND(SUM(cost_usd), 2) as TOTAL_COST,
    ROUND(AVG(qm.commit_msg_quality) * 100, 0) || '%' as AVG_QUALITY,
    substr(MIN(r.created_at), 1, 10) as FIRST_RUN,
    substr(MAX(r.created_at), 1, 10) as LAST_RUN
FROM runs r
JOIN quality_metrics qm ON r.id = qm.run_id
WHERE agent_name = 'alice'
GROUP BY agent_name
"

echo "\n${GREEN}✓ All 5 business questions answered from data!${NC}"
echo "\n${BLUE}Dashboard: ./bin/dandori dashboard${NC}"
