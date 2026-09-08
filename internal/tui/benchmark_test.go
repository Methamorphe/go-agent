package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/Methamorphe/go-agent/internal/workspace"
)

func BenchmarkG13Bound100KHistory(b *testing.B) {
	blocks := make([]workspace.Block, 100_000)
	for index := range blocks {
		blocks[index] = workspace.Block{ID: fmt.Sprintf("evt_%d", index), Preview: "synthetic projected block"}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		bounded := boundedBlocks(blocks, MaximumCacheBlocks)
		if len(bounded) != MaximumCacheBlocks {
			b.Fatal("unexpected bounded cache size")
		}
	}
}

func BenchmarkG13NormalViewportRedraw(b *testing.B) {
	model := NewModel(context.Background(), &fakeRuntimeClient{}, DefaultConfig())
	model.width, model.height = 140, 42
	model.loading = false
	model.rootID, model.focusID = "agt_root", "agt_root"
	model.blocks = make([]workspace.Block, DefaultCacheBlocks)
	for index := range model.blocks {
		model.blocks[index] = workspace.Block{ID: fmt.Sprintf("evt_%d", index), Kind: workspace.BlockToolResult, Preview: "go test ./... completed successfully with bounded output preview"}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = model.render()
	}
}
