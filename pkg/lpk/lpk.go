package lpk

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
	"github.com/zachfi/zkit/pkg/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

type ldapQuerier interface {
	query(ctx context.Context, username string) (*ldap.SearchResult, error)
}

type Lpk struct {
	logger  *slog.Logger
	tracer  trace.Tracer
	cfg     Config
	querier ldapQuerier
}

func New(cfg Config, logger *slog.Logger) (*Lpk, error) {
	l := &Lpk{
		cfg:    cfg,
		logger: logger,
		tracer: otel.Tracer("lpk"),
	}
	l.querier = &ldapClient{cfg: cfg, tracer: l.tracer, logger: logger}
	return l, nil
}

func (l *Lpk) Run(ctx context.Context, username string) error {
	var err error

	ctx, span := l.tracer.Start(ctx, "Lpk.Run",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("username", username),
			attribute.String(string(semconv.ServerAddressKey), l.cfg.Host),
		),
	)
	defer func() { _ = tracing.ErrHandler(span, err, "run failed", l.logger) }()

	results, queryErr := l.querier.query(ctx, username)
	if queryErr != nil {
		if l.cfg.CacheDir == "" {
			return queryErr
		}
		l.logger.Warn("ldap query failed, falling back to cache", "err", queryErr, "username", username)
		keys, cacheErr := l.readCache(username)
		if cacheErr != nil {
			// Return the original LDAP error so the caller knows what went wrong.
			return queryErr
		}
		for _, k := range keys {
			fmt.Printf("%s\n", k)
		}
		return nil
	}

	keys := extractKeys(results)

	if l.cfg.CacheDir != "" {
		if writeErr := l.writeCache(username, keys); writeErr != nil {
			l.logger.Warn("failed to write key cache", "err", writeErr, "username", username)
		}
	}

	for _, k := range keys {
		fmt.Printf("%s\n", k)
	}

	return nil
}

func (l *Lpk) cacheFile(username string) string {
	return filepath.Join(l.cfg.CacheDir, username)
}

func (l *Lpk) writeCache(username string, keys []string) error {
	if err := os.MkdirAll(l.cfg.CacheDir, 0o700); err != nil {
		return err
	}
	content := strings.Join(keys, "\n")
	if len(keys) > 0 {
		content += "\n"
	}
	return os.WriteFile(l.cacheFile(username), []byte(content), 0o600)
}

func (l *Lpk) readCache(username string) ([]string, error) {
	data, err := os.ReadFile(l.cacheFile(username))
	if err != nil {
		return nil, err
	}
	var keys []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if line != "" {
			keys = append(keys, line)
		}
	}
	return keys, nil
}

func extractKeys(results *ldap.SearchResult) []string {
	var keys []string
	for _, e := range results.Entries {
		for _, a := range e.Attributes {
			if a.Name == "sshPublicKey" {
				keys = append(keys, stringValues(a)...)
			}
		}
	}
	return keys
}

func stringValues(a *ldap.EntryAttribute) []string {
	var values []string
	for _, b := range a.ByteValues {
		values = append(values, string(b))
	}
	return values
}

// ldapClient implements ldapQuerier against a real LDAP server.
type ldapClient struct {
	cfg    Config
	tracer trace.Tracer
	logger *slog.Logger
}

func (c *ldapClient) query(ctx context.Context, username string) (*ldap.SearchResult, error) {
	var err error

	_, span := c.tracer.Start(ctx, "Lpk.query",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("username", username),
			attribute.String(string(semconv.ServerAddressKey), c.cfg.Host),
			attribute.String(string(semconv.DBSystemKey), "ldap"),
			attribute.String(string(semconv.DBNameKey), "ldap"),
		),
	)
	defer func() { _ = tracing.ErrHandler(span, err, "query failed", c.logger) }()

	tlsConfig := &tls.Config{}
	if c.cfg.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	ltls, err := ldap.DialURL(fmt.Sprintf("ldaps://%s:%d", c.cfg.Host, c.cfg.Port), ldap.DialWithTLSConfig(tlsConfig))
	if err != nil {
		return nil, err
	}

	ltls.SetTimeout(15 * time.Second)

	err = ltls.Bind(c.cfg.BindDN, c.cfg.BindPW)
	if err != nil {
		return nil, err
	}

	searchRequest := ldap.NewSearchRequest(
		c.cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(uid=%s)(sshPublicKey=*))", username),
		[]string{"sshPublicKey"},
		nil,
	)

	return ltls.Search(searchRequest)
}
