package health

import (
	"testing"
	"time"
)

func TestScenarios(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		m      Metrics
		score  int
		status string
	}{
		{"padaria saudavel", Metrics{"v5.2", now.Add(-2 * time.Hour), 450, 450, true, 12500, 10000, 1, 35}, 100, StatusLow},
		{"padaria backup corrompido", Metrics{"v5.2", now, 2, 450, false, 12500, 10000, 1, 35}, 65, StatusMedium},
		{"padaria crise", Metrics{"v5.1", now.Add(-7 * 24 * time.Hour), 450, 450, true, 6000, 10000, 7, 35}, 11, StatusHigh},
		{"alvorada", Metrics{"v5.0", now.Add(-7 * 24 * time.Hour), 300, 300, true, 60000, 100000, 8, 5}, 4, StatusHigh},
	}
	for _, c := range cases {
		r := Evaluate(c.m, now)
		if r.Score != c.score || r.Status != c.status {
			t.Errorf("%s: got %d/%s want %d/%s", c.name, r.Score, r.Status, c.score, c.status)
		}
		if len(r.Actions) == 0 {
			t.Errorf("%s: no actions", c.name)
		}
	}
}
