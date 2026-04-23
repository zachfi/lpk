package lpk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ldap "github.com/go-ldap/ldap/v3"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func noopTracer() trace.Tracer {
	return noop.NewTracerProvider().Tracer("test")
}

// fakeQuerier is an ldapQuerier that returns a fixed result or error.
type fakeQuerier struct {
	result *ldap.SearchResult
	err    error
}

func (f *fakeQuerier) query(_ context.Context, _ string) (*ldap.SearchResult, error) {
	return f.result, f.err
}

func makeEntry(keys ...string) *ldap.Entry {
	var byteVals [][]byte
	for _, k := range keys {
		byteVals = append(byteVals, []byte(k))
	}
	return &ldap.Entry{
		Attributes: []*ldap.EntryAttribute{
			{Name: "sshPublicKey", ByteValues: byteVals},
		},
	}
}

func newTestLpk(t *testing.T, cfg Config, q ldapQuerier) *Lpk {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	l := &Lpk{
		cfg:     cfg,
		logger:  logger,
		tracer:  noopTracer(),
		querier: q,
	}
	return l
}

func TestExtractKeys(t *testing.T) {
	result := &ldap.SearchResult{
		Entries: []*ldap.Entry{
			makeEntry("ssh-ed25519 AAAA1", "ssh-ed25519 AAAA2"),
		},
	}
	keys := extractKeys(result)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0] != "ssh-ed25519 AAAA1" || keys[1] != "ssh-ed25519 AAAA2" {
		t.Errorf("unexpected keys: %v", keys)
	}
}

func TestExtractKeys_NoSshPublicKey(t *testing.T) {
	result := &ldap.SearchResult{
		Entries: []*ldap.Entry{
			{Attributes: []*ldap.EntryAttribute{{Name: "cn", ByteValues: [][]byte{[]byte("alice")}}}},
		},
	}
	if got := extractKeys(result); len(got) != 0 {
		t.Errorf("expected no keys, got %v", got)
	}
}

func TestExtractKeys_EmptyResult(t *testing.T) {
	if got := extractKeys(&ldap.SearchResult{}); len(got) != 0 {
		t.Errorf("expected no keys, got %v", got)
	}
}

func TestWriteAndReadCache(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{CacheDir: dir}
	l := newTestLpk(t, cfg, &fakeQuerier{})

	keys := []string{"ssh-ed25519 AAAA1", "ssh-rsa BBBB2"}
	if err := l.writeCache("alice", keys); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	got, err := l.readCache("alice")
	if err != nil {
		t.Fatalf("readCache: %v", err)
	}
	if len(got) != len(keys) {
		t.Fatalf("expected %d keys, got %d", len(keys), len(got))
	}
	for i, k := range keys {
		if got[i] != k {
			t.Errorf("key[%d]: want %q, got %q", i, k, got[i])
		}
	}
}

func TestReadCache_Missing(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{CacheDir: dir}
	l := newTestLpk(t, cfg, &fakeQuerier{})

	_, err := l.readCache("nobody")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected ErrNotExist, got %v", err)
	}
}

func TestWriteCache_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "subdir", "lpk")
	cfg := Config{CacheDir: dir}
	l := newTestLpk(t, cfg, &fakeQuerier{})

	if err := l.writeCache("alice", []string{"ssh-ed25519 AAAA1"}); err != nil {
		t.Fatalf("writeCache: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected cache dir to be created: %v", err)
	}
}

func TestRun_WritesCache(t *testing.T) {
	dir := t.TempDir()
	key := "ssh-ed25519 AAAA_test_key"

	q := &fakeQuerier{
		result: &ldap.SearchResult{Entries: []*ldap.Entry{makeEntry(key)}},
	}
	cfg := Config{CacheDir: dir}
	l := newTestLpk(t, cfg, q)

	// Redirect stdout so we can capture the printed keys.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := l.Run(context.Background(), "alice")

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(string(out), key) {
		t.Errorf("expected key in stdout, got: %s", out)
	}

	cached, err := l.readCache("alice")
	if err != nil {
		t.Fatalf("readCache after successful Run: %v", err)
	}
	if len(cached) != 1 || cached[0] != key {
		t.Errorf("unexpected cached keys: %v", cached)
	}
}

func TestRun_FallsBackToCache(t *testing.T) {
	dir := t.TempDir()
	key := "ssh-ed25519 AAAA_cached"

	// Pre-populate cache.
	cacheFile := filepath.Join(dir, "alice")
	if err := os.WriteFile(cacheFile, []byte(key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	q := &fakeQuerier{err: fmt.Errorf("connection refused")}
	cfg := Config{CacheDir: dir}
	l := newTestLpk(t, cfg, q)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := l.Run(context.Background(), "alice")

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if err != nil {
		t.Fatalf("Run should succeed via cache, got: %v", err)
	}
	if !strings.Contains(string(out), key) {
		t.Errorf("expected cached key in stdout, got: %s", out)
	}
}

func TestRun_NoCacheDirReturnsError(t *testing.T) {
	q := &fakeQuerier{err: fmt.Errorf("connection refused")}
	cfg := Config{} // no CacheDir
	l := newTestLpk(t, cfg, q)

	if err := l.Run(context.Background(), "alice"); err == nil {
		t.Error("expected error when LDAP fails and no cache is configured")
	}
}

func TestRun_NoCaching_DoesNotWriteFile(t *testing.T) {
	key := "ssh-ed25519 AAAA_test"
	q := &fakeQuerier{
		result: &ldap.SearchResult{Entries: []*ldap.Entry{makeEntry(key)}},
	}
	cfg := Config{} // no CacheDir
	l := newTestLpk(t, cfg, q)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := l.Run(context.Background(), "alice")

	w.Close()
	os.Stdout = old
	io.ReadAll(r)

	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestRun_LDAPFailNoCacheFile(t *testing.T) {
	dir := t.TempDir()
	// Cache dir exists but no entry for "bob".
	q := &fakeQuerier{err: fmt.Errorf("ldap: connection timeout")}
	cfg := Config{CacheDir: dir}
	l := newTestLpk(t, cfg, q)

	if err := l.Run(context.Background(), "bob"); err == nil {
		t.Error("expected error when LDAP fails and no cache file exists for user")
	}
}
