package storage

import (
	"context"
	"time"
)

type Stats struct {
	OpenConnections int   `json:"open_connections"`
	InUse           int   `json:"in_use"`
	Idle            int   `json:"idle"`
	WaitCount       int64 `json:"wait_count"`
}

type ObjectRecord struct {
	Ref         string
	Algorithm   string
	Digest      string
	SizeBytes   int64
	MediaType   string
	Compression string
	CreatedAt   time.Time
}

type Store interface {
	Ping(context.Context) error

	Checkpoint(context.Context) error

	RegisterObject(
		context.Context,
		ObjectRecord,
	) error

	Object(
		context.Context,
		string,
	) (ObjectRecord, error)

	Stats() Stats

	Close() error
}
