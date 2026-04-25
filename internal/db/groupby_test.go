package db

import (
	"testing"
)

func seedGroupByRuns(t *testing.T, d *LocalDB) {
	t.Helper()
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}

	// Alice 5 · Bob 3 · Carol 2 (engineer)
	// Alice+Bob = Platform, Carol = Growth (department)
	rows := []struct {
		id, agent, engineer, dept string
		cost                      float64
	}{
		{"r-a1", "alpha", "Alice", "Platform", 1.0},
		{"r-a2", "alpha", "Alice", "Platform", 2.0},
		{"r-a3", "alpha", "Alice", "Platform", 1.5},
		{"r-a4", "alpha", "Alice", "Platform", 0.5},
		{"r-a5", "alpha", "Alice", "Platform", 0.5},
		{"r-b1", "", "Bob", "Platform", 0},
		{"r-b2", "", "Bob", "Platform", 0},
		{"r-b3", "", "Bob", "Platform", 0},
		{"r-c1", "beta", "Carol", "Growth", 3.0},
		{"r-c2", "beta", "Carol", "Growth", 4.0},
	}
	for _, r := range rows {
		var agent interface{}
		if r.agent != "" {
			agent = r.agent
		}
		_, err := d.Exec(`INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status, cost_usd, engineer_name, department)
			VALUES (?, ?, 'claude_code', 'seed', 'ws', datetime('now'), 'done', ?, ?, ?)`,
			r.id, agent, r.cost, r.engineer, r.dept)
		if err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}
}

func TestGroupBy_Engineer(t *testing.T) {
	d := newEmptyLocalDB(t)
	seedGroupByRuns(t, d)

	stats, err := d.GetCostByEngineer()
	if err != nil {
		t.Fatalf("GetCostByEngineer: %v", err)
	}
	if len(stats) != 3 {
		t.Fatalf("expected 3 groups, got %d (%+v)", len(stats), stats)
	}
	byName := map[string]int{}
	for _, s := range stats {
		byName[s.Group] = s.RunCount
	}
	if byName["Alice"] != 5 || byName["Bob"] != 3 || byName["Carol"] != 2 {
		t.Errorf("counts wrong: %+v", byName)
	}
}

func TestGroupBy_Department(t *testing.T) {
	d := newEmptyLocalDB(t)
	seedGroupByRuns(t, d)

	stats, err := d.GetCostByDepartment()
	if err != nil {
		t.Fatalf("GetCostByDepartment: %v", err)
	}
	byName := map[string]int{}
	for _, s := range stats {
		byName[s.Group] = s.RunCount
	}
	if byName["Platform"] != 8 {
		t.Errorf("Platform: expected 8, got %d", byName["Platform"])
	}
	if byName["Growth"] != 2 {
		t.Errorf("Growth: expected 2, got %d", byName["Growth"])
	}
}

func TestGroupBy_NullEngineer_ShownAsUnassigned(t *testing.T) {
	d := newEmptyLocalDB(t)
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}
	_, err := d.Exec(`INSERT INTO runs (id, agent_name, agent_type, user, workstation_id, started_at, status)
		VALUES ('r-u1', 'alpha', 'claude_code', 'seed', 'ws', datetime('now'), 'done')`)
	if err != nil {
		t.Fatal(err)
	}

	stats, err := d.GetCostByEngineer()
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Group != "(unassigned)" {
		t.Errorf("expected (unassigned), got %+v", stats)
	}
}

func TestMixLeaderboard_BlogTable(t *testing.T) {
	d := newEmptyLocalDB(t)
	seedGroupByRuns(t, d)

	rows, err := d.GetMixLeaderboard(30)
	if err != nil {
		t.Fatalf("GetMixLeaderboard: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (Alice+alpha, Bob human, Carol+beta), got %d: %+v", len(rows), rows)
	}

	var alice, bob, carol *MixRow
	for i := range rows {
		switch rows[i].Engineer {
		case "Alice":
			alice = &rows[i]
		case "Bob":
			bob = &rows[i]
		case "Carol":
			carol = &rows[i]
		}
	}
	if alice == nil || alice.Agent != "alpha" {
		t.Errorf("Alice row: expected agent=alpha, got %+v", alice)
	}
	if bob == nil || bob.Agent != "" {
		t.Errorf("Bob row: expected agent='' (human-only), got %+v", bob)
	}
	if carol == nil || carol.Agent != "beta" {
		t.Errorf("Carol row: expected agent=beta, got %+v", carol)
	}
}
