package soaktest

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultLatencySamples = 4096

type Config struct {
	Duration             time.Duration
	Interval             time.Duration
	SampleInterval       time.Duration
	MaxHeapGrowthBytes   uint64
	MaxGoroutineGrowth   int
	MaxLatencySamples    int
}

func ConfigFromEnv() (Config, error) {
	duration, err := DurationEnv("SOAK_DURATION", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	interval, err := DurationEnv("SOAK_INTERVAL", 250*time.Millisecond)
	if err != nil {
		return Config{}, err
	}
	sampleInterval, err := DurationEnv("SOAK_SAMPLE_INTERVAL", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	maxHeapGrowthMB, err := IntEnv("SOAK_MAX_HEAP_GROWTH_MB", 64)
	if err != nil {
		return Config{}, err
	}
	maxGoroutineGrowth, err := IntEnv("SOAK_MAX_GOROUTINE_GROWTH", 16)
	if err != nil {
		return Config{}, err
	}
	maxLatencySamples, err := IntEnv("SOAK_LATENCY_SAMPLES", defaultLatencySamples)
	if err != nil {
		return Config{}, err
	}

	if duration <= 0 {
		return Config{}, fmt.Errorf("SOAK_DURATION must be positive")
	}
	if interval <= 0 {
		return Config{}, fmt.Errorf("SOAK_INTERVAL must be positive")
	}
	if sampleInterval <= 0 {
		return Config{}, fmt.Errorf("SOAK_SAMPLE_INTERVAL must be positive")
	}
	if maxHeapGrowthMB < 0 {
		return Config{}, fmt.Errorf("SOAK_MAX_HEAP_GROWTH_MB must be non-negative")
	}
	if maxGoroutineGrowth < 0 {
		return Config{}, fmt.Errorf("SOAK_MAX_GOROUTINE_GROWTH must be non-negative")
	}
	if maxLatencySamples <= 0 {
		return Config{}, fmt.Errorf("SOAK_LATENCY_SAMPLES must be positive")
	}

	return Config{
		Duration:           duration,
		Interval:           interval,
		SampleInterval:     sampleInterval,
		MaxHeapGrowthBytes: uint64(maxHeapGrowthMB) << 20,
		MaxGoroutineGrowth: maxGoroutineGrowth,
		MaxLatencySamples:  maxLatencySamples,
	}, nil
}

func DurationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func IntEnv(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

type RuntimeSample struct {
	At         time.Time
	HeapAlloc  uint64
	HeapInuse  uint64
	HeapObjects uint64
	NumGC      uint32
	Goroutines int
}

func SampleRuntime() RuntimeSample {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return RuntimeSample{
		At:          time.Now().UTC(),
		HeapAlloc:   stats.HeapAlloc,
		HeapInuse:   stats.HeapInuse,
		HeapObjects: stats.HeapObjects,
		NumGC:       stats.NumGC,
		Goroutines:  runtime.NumGoroutine(),
	}
}

func SampleRuntimeAfterGC() RuntimeSample {
	runtime.GC()
	return SampleRuntime()
}

func ValidateRuntimeGrowth(baseline, final RuntimeSample, cfg Config) error {
	if final.HeapAlloc > baseline.HeapAlloc+cfg.MaxHeapGrowthBytes {
		return fmt.Errorf(
			"heap growth exceeded bound: baseline=%d final=%d allowed_growth=%d",
			baseline.HeapAlloc,
			final.HeapAlloc,
			cfg.MaxHeapGrowthBytes,
		)
	}
	if final.Goroutines > baseline.Goroutines+cfg.MaxGoroutineGrowth {
		return fmt.Errorf(
			"goroutine growth exceeded bound: baseline=%d final=%d allowed_growth=%d",
			baseline.Goroutines,
			final.Goroutines,
			cfg.MaxGoroutineGrowth,
		)
	}
	return nil
}

type LatencyWindow struct {
	values []time.Duration
	next   int
	full   bool
	max    time.Duration
}

func NewLatencyWindow(capacity int) *LatencyWindow {
	if capacity <= 0 {
		capacity = defaultLatencySamples
	}
	return &LatencyWindow{values: make([]time.Duration, capacity)}
}

func (w *LatencyWindow) Add(value time.Duration) {
	if len(w.values) == 0 {
		return
	}
	w.values[w.next] = value
	w.next++
	if w.next == len(w.values) {
		w.next = 0
		w.full = true
	}
	if value > w.max {
		w.max = value
	}
}

func (w *LatencyWindow) Count() int {
	if w.full {
		return len(w.values)
	}
	return w.next
}

func (w *LatencyWindow) P95() time.Duration {
	count := w.Count()
	if count == 0 {
		return 0
	}
	copyValues := append([]time.Duration(nil), w.values[:count]...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	index := (95*len(copyValues) + 99) / 100
	if index <= 0 {
		index = 1
	}
	if index > len(copyValues) {
		index = len(copyValues)
	}
	return copyValues[index-1]
}

func (w *LatencyWindow) Max() time.Duration {
	return w.max
}
