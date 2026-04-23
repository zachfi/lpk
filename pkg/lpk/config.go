package lpk

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/zachfi/zkit/pkg/tracing"
	"gopkg.in/yaml.v2"
)

type Config struct {
	BindDN             string         `yaml:"binddn,omitempty"`
	BindPW             string         `yaml:"bindpw,omitempty"`
	BaseDN             string         `yaml:"basedn,omitempty"`
	Host               string         `yaml:"host,omitempty"`
	Port               int            `yaml:"port,omitempty"`
	InsecureSkipVerify bool           `yaml:"insecure_skip_verify,omitempty"`
	// CacheDir is the directory used to persist SSH public keys after a
	// successful LDAP lookup. When LDAP is unreachable (e.g. during boot),
	// the cached keys are served instead. Leave empty to disable caching.
	CacheDir string         `yaml:"cache_dir,omitempty"`
	Tracing  tracing.Config `yaml:"tracing"`
}

// LoadConfig receives a file path for a configuration to load.
func LoadConfig(file string) (Config, error) {
	filename, _ := filepath.Abs(file)

	config := Config{}
	err := loadYamlFile(filename, &config)
	if err != nil {
		return config, errors.Wrap(err, "failed to load yaml file")
	}

	return config, nil
}

// loadYamlFile unmarshals a YAML file into the received value or returns an error.
func loadYamlFile(filename string, d any) error {
	yamlFile, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(yamlFile, d)
	if err != nil {
		return err
	}

	return nil
}

func (c *Config) RegisterFlagsAndApplyDefaults(prefix string, f *flag.FlagSet) {
	f.IntVar(&c.Port, "port", 636, "ldap connection port")
	f.StringVar(&c.CacheDir, "cache-dir", "", "directory for caching SSH keys (empty disables caching)")
}
