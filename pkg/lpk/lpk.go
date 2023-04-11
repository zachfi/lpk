package lpk

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/go-kit/log"
	ldap "github.com/go-ldap/ldap/v3"
	"github.com/zachfi/znet/pkg/util"
)

type Lpk struct {
	cfg    Config
	logger log.Logger
}

func New(cfg Config) (*Lpk, error) {
	z := &Lpk{
		cfg: cfg,
	}

	z.logger = util.NewLogger()

	return z, nil
}

func (l *Lpk) Run(username string) error {
	tlsConfig := &tls.Config{}

	if l.cfg.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	ltls, err := ldap.DialTLS(
		"tcp",
		fmt.Sprintf("%s:%d", l.cfg.Host, l.cfg.Port),
		tlsConfig,
	)
	if err != nil {
		return err
	}

	ltls.SetTimeout(15 * time.Second)

	err = ltls.Bind(l.cfg.BindDN, l.cfg.BindPW)
	if err != nil {
		return err
	}

	searchRequest := ldap.NewSearchRequest(
		l.cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(uid=%s)(sshPublicKey=*))", username),
		[]string{"sshPublicKey"},
		nil,
	)

	searchResult, err := ltls.Search(searchRequest)
	if err != nil {
		return err
	}

	var keys []string

	for _, e := range searchResult.Entries {
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

func stringValues(a *ldap.EntryAttribute) []string {
	var values []string

	for _, b := range a.ByteValues {
		values = append(values, string(b))
	}

	return values
}
