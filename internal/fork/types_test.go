package fork

import (
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

func TestG8SiblingOperationNamespacesCannotCollide(t *testing.T) {
	action := id.ActionID("act_same_logical_step")
	a := NamespaceAction(id.ForkID("frk_a"), action)
	b := NamespaceAction(id.ForkID("frk_b"), action)
	if a == b || a == action || b == action { t.Fatalf("namespaces collided: a=%q b=%q base=%q", a, b, action) }
}

func TestG8ObjectiveEvaluatorSelectsBestEligibleBranchDeterministically(t *testing.T) {
	now := time.Unix(100, 0)
	objective := Objective{CorrectnessWeight: 0.7, QualityWeight: 0.2, LatencyWeight: 0.1, MinCorrectness: 1}
	aEval := Evaluate(Metrics{Correctness: 1, Quality: 0.9, LatencyMS: 100}, objective, now)
	bEval := Evaluate(Metrics{Correctness: 1, Quality: 0.9, LatencyMS: 20}, objective, now)
	badEval := Evaluate(Metrics{Correctness: 0.8, Quality: 1, LatencyMS: 1}, objective, now)
	branches := []Branch{{ID: id.ForkID("frk_a"), Evaluation: &aEval}, {ID: id.ForkID("frk_b"), Evaluation: &bEval}, {ID: id.ForkID("frk_bad"), Evaluation: &badEval}}
	winner, err := SelectWinner(branches)
	if err != nil { t.Fatal(err) }
	if winner != id.ForkID("frk_b") { t.Fatalf("winner=%s want frk_b", winner) }
	if bEval.Reason == "" || badEval.Eligible { t.Fatalf("evaluation evidence incomplete: good=%+v bad=%+v", bEval, badEval) }
}
