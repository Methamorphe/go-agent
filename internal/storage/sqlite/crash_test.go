package sqlite

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/storage"
)

func TestRecoveryAfterHardKilledWriter(
	t *testing.T,
) {
	path :=
		t.TempDir() +
			string(os.PathSeparator) +
			"runtime.db"

	cmd := exec.Command(
		os.Args[0],
		"-test.run=TestSQLiteCrashHelper",
	)

	cmd.Env = append(
		os.Environ(),
		"GO_AGENT_CRASH_HELPER=1",
		"GO_AGENT_CRASH_DB="+path,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	scanner := bufio.NewScanner(stdout)

	if !scanner.Scan() ||
		scanner.Text() != "READY" {
		_ = cmd.Process.Kill()

		t.Fatalf(
			"helper failed to become ready: %q",
			scanner.Text(),
		)
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}

	_ = cmd.Wait()

	store, err := Open(
		context.Background(),
		Config{
			Path: path,
		},
	)

	if err != nil {
		t.Fatalf(
			"reopen after hard kill: %v",
			err,
		)
	}

	defer store.Close()

	_, err = store.Object(
		context.Background(),
		crashObject().Ref,
	)

	if err != nil {
		t.Fatalf(
			"read committed metadata after hard kill: %v",
			err,
		)
	}
}

func TestSQLiteCrashHelper(
	t *testing.T,
) {
	if os.Getenv(
		"GO_AGENT_CRASH_HELPER",
	) != "1" {
		return
	}

	store, err := Open(
		context.Background(),
		Config{
			Path: os.Getenv(
				"GO_AGENT_CRASH_DB",
			),
		},
	)

	if err != nil {
		os.Exit(10)
	}

	if err := store.RegisterObject(
		context.Background(),
		crashObject(),
	); err != nil {
		os.Exit(11)
	}

	fmt.Println("READY")

	select {}
}

func crashObject() storage.ObjectRecord {
	const digest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	return storage.ObjectRecord{
		Ref:         "sha256:" + digest,
		Algorithm:   "sha256",
		Digest:      digest,
		SizeBytes:   42,
		Compression: "none",

		CreatedAt: time.Date(
			2026,
			time.August,
			27,
			10,
			0,
			0,
			0,
			time.UTC,
		),
	}
}
