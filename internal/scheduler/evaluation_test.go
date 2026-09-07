package scheduler

import (
	"testing"
	"time"
)

func TestG6EvaluationAgainstStaticBaselines(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	strongest := testProfile("cloud", "strongest", LocalityCloud, .99, 4*time.Second)
	strongest.InputCostMicrosPerMillion = 80_000_000
	strongest.OutputCostMicrosPerMillion = 160_000_000
	cheapest := testProfile("cloud", "cheapest", LocalityCloud, .78, 2*time.Second)
	cheapest.InputCostMicrosPerMillion = 100_000
	cheapest.OutputCostMicrosPerMillion = 200_000
	fastest := testProfile("local", "fastest", LocalityLocal, .75, 100*time.Millisecond)
	fastest.InputCostMicrosPerMillion = 1_000_000
	fastest.OutputCostMicrosPerMillion = 1_000_000

	router := routerWith(t, clock, strongest, cheapest, fastest)
	task := testTask()
	task.Budget = Resources{MoneyMicros: 1_000_000, Tokens: 10_000}

	task.Objective = ObjectiveQualityFirst
	quality, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	if quality.Selected != strongest.Ref() {
		t.Fatalf("quality-first selected %s, strongest baseline=%s", quality.Selected.Key(), strongest.Key())
	}

	task.Objective = ObjectiveCostFirst
	cost, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cost.Selected != cheapest.Ref() {
		t.Fatalf("cost-first selected %s, cheapest baseline=%s", cost.Selected.Key(), cheapest.Key())
	}

	task.Objective = ObjectiveLatencyFirst
	latency, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	if latency.Selected != fastest.Ref() {
		t.Fatalf("latency-first selected %s, fastest baseline=%s", latency.Selected.Key(), fastest.Key())
	}

	task.Objective = ObjectiveBalanced
	balanced, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	if balanced.Selected == (ModelRef{}) {
		t.Fatal("balanced policy produced no model")
	}
	if balanced.Selected == strongest.Ref() && balanced.EstimatedCost.MoneyMicros >= cost.EstimatedCost.MoneyMicros {
		t.Fatal("balanced policy collapsed to always-strongest without a cost tradeoff")
	}
	if balanced.Selected == cheapest.Ref() && balanced.Score.Quality < latency.Score.Quality {
		t.Fatal("balanced policy collapsed to always-cheapest despite quality loss")
	}
}
