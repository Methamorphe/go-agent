package objectstore

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestPutOpenVerify(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	meta, err := store.Put(
		context.Background(),
		strings.NewReader("hello"),
	)

	if err != nil {
		t.Fatal(err)
	}

	const want Ref = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

	if meta.Ref != want {
		t.Fatalf(
			"ref = %s, want %s",
			meta.Ref,
			want,
		)
	}

	reader, err :=
		store.Open(meta.Ref)

	if err != nil {
		t.Fatal(err)
	}

	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if string(body) != "hello" {
		t.Fatalf(
			"body = %q",
			body,
		)
	}

	if err := store.Verify(
		context.Background(),
		meta.Ref,
	); err != nil {
		t.Fatal(err)
	}
}
