package lpk

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

type Lpk struct {
	logger *slog.Logger
	tracer trace.Tracer
	cfg    Config
}

func New(cfg Config, logger *slog.Logger) (*Lpk, error) {
	l := &Lpk{
		cfg:    cfg,
		logger: logger,
		tracer: otel.Tracer("lpk"),
	}

	return l, nil
}

func (l *Lpk) Run(ctx context.Context, username string) error {
	_, span := l.tracer.Start(ctx, "Lpk.Run",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("username", username),
			attribute.String(string(semconv.ServerAddressKey), l.cfg.Host),
		),
	)
	defer span.End()

	results, err := l.query(ctx, username)
	if err != nil {
		return err
	}

	var keys []string
	for _, e := range results.Entries {
		for _, a := range e.Attributes {
			switch a.Name {
			case "sshPublicKey":
				keys = append(keys, stringValues(a)...)
			}
		}
	}

	for _, k := range keys {
		fmt.Printf("%s\n", k)
	}

	return nil
}

func (l *Lpk) query(ctx context.Context, username string) (*ldap.SearchResult, error) {
	_, span := l.tracer.Start(ctx, "Lpk.query",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("username", username),
			attribute.String(string(semconv.ServerAddressKey), l.cfg.Host),
			// attribute.String(string(semconv.NetworkPeerAddressKey), l.cfg.Host),
			// attribute.Int(string(semconv.NetworkPeerPortKey), l.cfg.Port),
			attribute.String(string(semconv.DBSystemKey), "ldap"),
		),
	)
	defer span.End()

	tlsConfig := &tls.Config{}
	if l.cfg.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	ltls, err := ldap.DialURL(fmt.Sprintf("ldaps://%s:%d", l.cfg.Host, l.cfg.Port), ldap.DialWithTLSConfig(tlsConfig))
	if err != nil {
		return nil, err
	}

	ltls.SetTimeout(15 * time.Second)

	err = ltls.Bind(l.cfg.BindDN, l.cfg.BindPW)
	if err != nil {
		return nil, err
	}

	searchRequest := ldap.NewSearchRequest(
		l.cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(uid=%s)(sshPublicKey=*))", username),
		[]string{"sshPublicKey"},
		nil,
	)

	return ltls.Search(searchRequest)
}

func stringValues(a *ldap.EntryAttribute) []string {
	var values []string

	for _, b := range a.ByteValues {
		values = append(values, string(b))
	}

	return values
}
