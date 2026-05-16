package migrate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type fakeSchemaRow struct {
	value int64
	dirty bool
	err   error
}

func (r fakeSchemaRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	switch d := dest[0].(type) {
	case *int64:
		*d = r.value
	case *bool:
		*d = r.dirty
	default:
		return errors.New("unsupported scan target")
	}
	return nil
}

type fakeSchemaQuerier struct {
	version int64
	dirty   bool
}

func (q fakeSchemaQuerier) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	if strings.Contains(sql, "MAX(version)") {
		return fakeSchemaRow{value: q.version}
	}
	return fakeSchemaRow{dirty: q.dirty}
}

func TestReadSchemaVersionNormalizesFinalizedProvisionals(t *testing.T) {
	version, err := ReadSchemaVersion(context.Background(), fakeSchemaQuerier{version: 900001})
	if err != nil {
		t.Fatalf("ReadSchemaVersion returned error: %v", err)
	}
	if version != 371 {
		t.Fatalf("version = %d, want 371", version)
	}
}

func TestReadSchemaVersionRejectsUnknownProvisionals(t *testing.T) {
	_, err := ReadSchemaVersion(context.Background(), fakeSchemaQuerier{version: 900123})
	if err == nil {
		t.Fatal("ReadSchemaVersion should reject unknown provisional versions")
	}
	if !strings.Contains(err.Error(), "unfinalized provisional version 900123") {
		t.Fatalf("error = %v", err)
	}
}

func TestReadSchemaVersionRejectsDirtyStateBeforeNormalization(t *testing.T) {
	_, err := ReadSchemaVersion(context.Background(), fakeSchemaQuerier{version: 900001, dirty: true})
	if err == nil {
		t.Fatal("ReadSchemaVersion should reject dirty provisional aliases")
	}
	if !strings.Contains(err.Error(), "migration version 900001 is dirty") {
		t.Fatalf("error = %v", err)
	}
}
