package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
)

type Ref string

type Meta struct {
	Ref       Ref    `json:"ref"`
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type Store struct {
	root    string
	tempDir string
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errs.New(
			errs.CodeInvalidArgument,
			"objectstore.new",
			"root path is empty",
		)
	}

	root, err := filepath.Abs(
		filepath.Clean(root),
	)
	if err != nil {
		return nil, errs.Wrap(
			errs.CodeInvalidArgument,
			"objectstore.new",
			"normalize root",
			err,
		)
	}

	tempDir := filepath.Join(
		root,
		".tmp",
	)

	if err := os.MkdirAll(
		tempDir,
		0o700,
	); err != nil {
		return nil, errs.Wrap(
			errs.CodeUnavailable,
			"objectstore.new",
			"create object directories",
			err,
		)
	}

	return &Store{
		root:    root,
		tempDir: tempDir,
	}, nil
}

func ParseRef(value string) (Ref, error) {
	algorithm, digest, ok := strings.Cut(
		value,
		":",
	)

	if !ok ||
		algorithm != "sha256" ||
		len(digest) != sha256.Size*2 {
		return "", errs.New(
			errs.CodeInvalidArgument,
			"objectstore.parse_ref",
			"invalid sha256 object reference",
		)
	}

	decoded, err := hex.DecodeString(digest)
	if err != nil ||
		len(decoded) != sha256.Size {
		return "", errs.New(
			errs.CodeInvalidArgument,
			"objectstore.parse_ref",
			"invalid sha256 digest",
		)
	}

	return Ref(value), nil
}

func (s *Store) Put(
	ctx context.Context,
	src io.Reader,
) (Meta, error) {
	if src == nil {
		return Meta{}, errs.New(
			errs.CodeInvalidArgument,
			"objectstore.put",
			"reader is nil",
		)
	}

	temp, err := os.CreateTemp(
		s.tempDir,
		"obj-*",
	)
	if err != nil {
		return Meta{}, errs.Wrap(
			errs.CodeUnavailable,
			"objectstore.put",
			"create temporary object",
			err,
		)
	}

	tempPath := temp.Name()

	closed := false
	cleanup := true

	defer func() {
		if !closed {
			_ = temp.Close()
		}

		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	hash := sha256.New()

	size, err := copyContext(
		ctx,
		io.MultiWriter(temp, hash),
		src,
	)
	if err != nil {
		return Meta{}, errs.Wrap(
			errs.CodeUnavailable,
			"objectstore.put",
			"stream object",
			err,
		)
	}

	if err := temp.Sync(); err != nil {
		return Meta{}, errs.Wrap(
			errs.CodeUnavailable,
			"objectstore.put",
			"sync temporary object",
			err,
		)
	}

	if err := temp.Close(); err != nil {
		closed = true

		return Meta{}, errs.Wrap(
			errs.CodeUnavailable,
			"objectstore.put",
			"close temporary object",
			err,
		)
	}

	closed = true

	digest := hex.EncodeToString(
		hash.Sum(nil),
	)

	ref := Ref(
		"sha256:" + digest,
	)

	finalPath := s.pathForDigest(digest)

	if err := os.MkdirAll(
		filepath.Dir(finalPath),
		0o700,
	); err != nil {
		return Meta{}, errs.Wrap(
			errs.CodeUnavailable,
			"objectstore.put",
			"create final directory",
			err,
		)
	}

	if err := os.Rename(
		tempPath,
		finalPath,
	); err != nil {
		if _, statErr := os.Stat(finalPath); statErr != nil {
			return Meta{}, errs.Wrap(
				errs.CodeUnavailable,
				"objectstore.put",
				"finalize object",
				err,
			)
		}

		if removeErr := os.Remove(
			tempPath,
		); removeErr != nil &&
			!os.IsNotExist(removeErr) {
			return Meta{}, errs.Wrap(
				errs.CodeUnavailable,
				"objectstore.put",
				"remove duplicate temporary object",
				removeErr,
			)
		}
	}

	cleanup = false

	if err := syncDirectory(
		filepath.Dir(finalPath),
	); err != nil {
		return Meta{}, errs.Wrap(
			errs.CodeUnavailable,
			"objectstore.put",
			"sync object directory",
			err,
		)
	}

	return Meta{
		Ref:       ref,
		Algorithm: "sha256",
		Digest:    digest,
		Size:      size,
	}, nil
}

func (s *Store) Open(
	ref Ref,
) (io.ReadCloser, error) {
	parsed, err := ParseRef(string(ref))
	if err != nil {
		return nil, err
	}

	_, digest, _ := strings.Cut(
		string(parsed),
		":",
	)

	file, err := os.Open(
		s.pathForDigest(digest),
	)

	if os.IsNotExist(err) {
		return nil, errs.Wrap(
			errs.CodeNotFound,
			"objectstore.open",
			"object not found",
			err,
		)
	}

	if err != nil {
		return nil, errs.Wrap(
			errs.CodeUnavailable,
			"objectstore.open",
			"open object",
			err,
		)
	}

	return file, nil
}

func (s *Store) Verify(
	ctx context.Context,
	ref Ref,
) error {
	reader, err := s.Open(ref)
	if err != nil {
		return err
	}

	defer reader.Close()

	hash := sha256.New()

	if _, err := copyContext(
		ctx,
		hash,
		reader,
	); err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"objectstore.verify",
			"read object",
			err,
		)
	}

	actual := "sha256:" +
		hex.EncodeToString(hash.Sum(nil))

	if actual != string(ref) {
		return errs.New(
			errs.CodeCorruption,
			"objectstore.verify",
			fmt.Sprintf(
				"hash mismatch: got %s",
				actual,
			),
		)
	}

	return nil
}

func (s *Store) CleanupTemps(
	now time.Time,
	olderThan time.Duration,
) (int, error) {
	entries, err := os.ReadDir(s.tempDir)
	if err != nil {
		return 0, errs.Wrap(
			errs.CodeUnavailable,
			"objectstore.cleanup",
			"read temp directory",
			err,
		)
	}

	cutoff := now.Add(-olderThan)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() ||
			!strings.HasPrefix(
				entry.Name(),
				"obj-",
			) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return removed, errs.Wrap(
				errs.CodeUnavailable,
				"objectstore.cleanup",
				"stat temp object",
				err,
			)
		}

		if info.ModTime().After(cutoff) {
			continue
		}

		err = os.Remove(
			filepath.Join(
				s.tempDir,
				entry.Name(),
			),
		)

		if err != nil &&
			!os.IsNotExist(err) {
			return removed, errs.Wrap(
				errs.CodeUnavailable,
				"objectstore.cleanup",
				"remove temp object",
				err,
			)
		}

		removed++
	}

	return removed, nil
}

func (s *Store) pathForDigest(
	digest string,
) string {
	return filepath.Join(
		s.root,
		"sha256",
		digest[:2],
		digest[2:4],
		digest,
	)
}

func copyContext(
	ctx context.Context,
	dst io.Writer,
	src io.Reader,
) (int64, error) {
	buffer := make([]byte, 64*1024)

	var total int64

	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}

		n, readErr := src.Read(buffer)

		if n > 0 {
			written, writeErr :=
				dst.Write(buffer[:n])

			total += int64(written)

			if writeErr != nil {
				return total, writeErr
			}

			if written != n {
				return total, io.ErrShortWrite
			}
		}

		if readErr == io.EOF {
			return total, nil
		}

		if readErr != nil {
			return total, readErr
		}
	}
}
