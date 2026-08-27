package diag

import (
	"runtime"
	"time"
)

type RuntimeSnapshot struct {
	Timestamp       time.Time `json:"timestamp"`
	GoVersion       string    `json:"go_version"`
	GOOS            string    `json:"goos"`
	GOARCH          string    `json:"goarch"`
	NumCPU          int       `json:"num_cpu"`
	Goroutines      int       `json:"goroutines"`
	HeapAllocBytes  uint64    `json:"heap_alloc_bytes"`
	HeapObjects     uint64    `json:"heap_objects"`
	TotalAllocBytes uint64    `json:"total_alloc_bytes"`
	SysBytes        uint64    `json:"sys_bytes"`
}

func Snapshot(
	now time.Time,
) RuntimeSnapshot {
	var memory runtime.MemStats

	runtime.ReadMemStats(&memory)

	return RuntimeSnapshot{
		Timestamp:       now.UTC(),
		GoVersion:       runtime.Version(),
		GOOS:            runtime.GOOS,
		GOARCH:          runtime.GOARCH,
		NumCPU:          runtime.NumCPU(),
		Goroutines:      runtime.NumGoroutine(),
		HeapAllocBytes:  memory.HeapAlloc,
		HeapObjects:     memory.HeapObjects,
		TotalAllocBytes: memory.TotalAlloc,
		SysBytes:        memory.Sys,
	}
}
