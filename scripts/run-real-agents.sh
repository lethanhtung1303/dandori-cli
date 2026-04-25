#!/bin/zsh
# Run real Claude agents with different names
# Limited runs to control costs

set -e

DANDORI="/Users/phucnt/workspace/dandori-workspace/dandori-cli/bin/dandori"
CONFIG="$HOME/.dandori/config.yaml"
TEST_WS="/tmp/dandori-real-test"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
echo "${BLUE}║         Real Agent Runs (Cost-Controlled)              ║${NC}"
echo "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"

# Setup
rm -f ~/.dandori/local.db
$DANDORI init --no-shell 2>/dev/null || true

rm -rf "$TEST_WS"
mkdir -p "$TEST_WS"
cd "$TEST_WS"
git init -q
git config user.email "test@dandori.dev"
git config user.name "Test"
cat > main.go << 'EOF'
package main

import "fmt"

func main() {
    fmt.Println("Hello")
}
EOF
git add . && git commit -q -m "init"

# Function to set agent name
set_agent() {
    sed -i '' "s/name: .*/name: $1/" "$CONFIG"
    echo "${YELLOW}Agent: $1${NC}"
}

# Function to run agent
run_agent() {
    local prompt=$1
    echo "  Prompt: $prompt"
    $DANDORI run -- claude -p "$prompt" --allowedTools Read,Bash,Write,Edit 2>&1 | tail -3
    echo "${GREEN}  ✓ Done${NC}"
    sleep 2
}

# Run 1: alice - read file
echo "\n${BLUE}=== Run 1/8 ===${NC}"
set_agent "alice"
run_agent "Read main.go and tell me what it does in one sentence"

# Run 2: bob - list files
echo "\n${BLUE}=== Run 2/8 ===${NC}"
set_agent "bob"
run_agent "Run ls -la and summarize what you see in 2 lines"

# Run 3: alice - add comment
echo "\n${BLUE}=== Run 3/8 ===${NC}"
set_agent "alice"
run_agent "Add a comment to main.go explaining what it does"

# Run 4: charlie - check git
echo "\n${BLUE}=== Run 4/8 ===${NC}"
set_agent "charlie"
run_agent "Run git log --oneline and tell me the commits"

# Run 5: david - create file
echo "\n${BLUE}=== Run 5/8 ===${NC}"
set_agent "david"
run_agent "Create a file called README.md with project title only"

# Run 6: bob - run go
echo "\n${BLUE}=== Run 6/8 ===${NC}"
set_agent "bob"
run_agent "Run go run main.go and show the output"

# Run 7: alice - refactor
echo "\n${BLUE}=== Run 7/8 ===${NC}"
set_agent "alice"
run_agent "Add a helper function to main.go that returns 'world'"

# Run 8: eve - review
echo "\n${BLUE}=== Run 8/8 ===${NC}"
set_agent "eve"
run_agent "Read main.go and rate the code quality 1-10 with one reason"

# Restore original agent
set_agent "alpha"

# Summary
echo "\n${BLUE}========== SUMMARY ==========${NC}"
sqlite3 -header -column ~/.dandori/local.db "
SELECT
    agent_name,
    COUNT(*) as runs,
    ROUND(SUM(cost_usd), 4) as cost,
    SUM(input_tokens + output_tokens) as tokens
FROM runs
GROUP BY agent_name
ORDER BY runs DESC
"

echo "\n${GREEN}✓ Real data generated!${NC}"
echo "Dashboard: $DANDORI dashboard"
