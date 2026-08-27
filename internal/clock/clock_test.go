package clock

import (
	"testing"
	"time"
)

func TestFakeClockReturnsInitialTime(t *testing.T) {
	initial := time.Date(
		2026,
		time.January,
		2,
		3,
		4,
		5,
		0,
		time.UTC,
	)

	clock := NewFakeClock(initial)

	got := clock.Now()

	if !got.Equal(initial) {
		t.Fatalf(
			"Now() = %v, want %v",
			got,
			initial,
		)
	}
}

func TestFakeClockAdvance(t *testing.T) {
	initial := time.Date(
		2026,
		time.January,
		2,
		3,
		4,
		5,
		0,
		time.UTC,
	)

	clock := NewFakeClock(initial)

	clock.Advance(10 * time.Minute)

	want := initial.Add(10 * time.Minute)
	got := clock.Now()

	if !got.Equal(want) {
		t.Fatalf(
			"Now() = %v, want %v",
			got,
			want,
		)
	}
}

func TestFakeClockSetNormalizesToUTC(t *testing.T) {
	location := time.FixedZone("UTC+2", 2*60*60)

	local := time.Date(
		2026,
		time.August,
		27,
		12,
		0,
		0,
		0,
		location,
	)

	clock := NewFakeClock(time.Time{})

	clock.Set(local)

	got := clock.Now()

	if got.Location() != time.UTC {
		t.Fatalf(
			"Now() location = %v, want UTC",
			got.Location(),
		)
	}

	want := local.UTC()

	if !got.Equal(want) {
		t.Fatalf(
			"Now() = %v, want %v",
			got,
			want,
		)
	}
}
