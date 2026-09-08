package fork

import (
	"reflect"
	"testing"

	"github.com/Methamorphe/go-agent/internal/world"
)

func TestG8IntersectIntentNeverBroadensHistoricalOrCurrentAuthority(t *testing.T) {
	historical := world.Intent{
		Version:            4,
		Goal:               "historical goal",
		AcceptanceCriteria: []string{"tests pass"},
		AllowedDomains:     []string{"fs.read", "fs.write"},
		ForbiddenDomains:   []string{"network"},
	}
	current := world.Intent{
		Version:            9,
		Goal:               "current goal",
		AcceptanceCriteria: []string{"security passes"},
		AllowedDomains:     []string{"fs.write", "network"},
		ForbiddenDomains:   []string{"process.exec"},
	}

	got := intersectIntent(historical, current)
	if got.Version != current.Version {
		t.Fatalf("version=%d want current %d", got.Version, current.Version)
	}
	if got.Goal != current.Goal {
		t.Fatalf("goal=%q want current goal %q", got.Goal, current.Goal)
	}
	if want := []string{"fs.write"}; !reflect.DeepEqual(got.AllowedDomains, want) {
		t.Fatalf("allowed=%v want %v", got.AllowedDomains, want)
	}
	if want := []string{"network", "process.exec"}; !reflect.DeepEqual(got.ForbiddenDomains, want) {
		t.Fatalf("forbidden=%v want %v", got.ForbiddenDomains, want)
	}
	if want := []string{"security passes", "tests pass"}; !reflect.DeepEqual(got.AcceptanceCriteria, want) {
		t.Fatalf("criteria=%v want %v", got.AcceptanceCriteria, want)
	}
}

func TestG8IntersectIntentTreatsEmptyAllowlistAsUnrestricted(t *testing.T) {
	got := intersectIntent(
		world.Intent{AllowedDomains: nil},
		world.Intent{Version: 2, AllowedDomains: []string{"fs.read"}},
	)
	if want := []string{"fs.read"}; !reflect.DeepEqual(got.AllowedDomains, want) {
		t.Fatalf("historical unrestricted intersection=%v want %v", got.AllowedDomains, want)
	}

	got = intersectIntent(
		world.Intent{AllowedDomains: []string{"fs.write"}},
		world.Intent{Version: 3, AllowedDomains: nil},
	)
	if want := []string{"fs.write"}; !reflect.DeepEqual(got.AllowedDomains, want) {
		t.Fatalf("current unrestricted intersection=%v want %v", got.AllowedDomains, want)
	}
}
