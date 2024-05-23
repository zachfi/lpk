// Copyright © 2022 Zach Leslie <code@zleslie.info>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/grafana/dskit/flagext"
	"github.com/zachfi/lpk/pkg/lpk"
	"github.com/zachfi/zkit/pkg/tracing"
	yaml "gopkg.in/yaml.v2"
)

var username string

var (
	goos      = "unknown"
	goarch    = "unknown"
	gitCommit = "$Format:%H$" // sha1 from git, output of $(git rev-parse HEAD)

	buildDate = "1970-01-01T00:00:00Z" // build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
)

// version contains all the information related to the CLI version
type version struct {
	GitCommit string `json:"gitCommit"`
	BuildDate string `json:"buildDate"`
	GoOs      string `json:"goOs"`
	GoArch    string `json:"goArch"`
}

// versionString returns the CLI version
func versionString() string {
	return fmt.Sprintf("Version: %#v", version{
		gitCommit,
		buildDate,
		goos,
		goarch,
	})
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("failed to load config file", "err", err)
		os.Exit(1)
	}

	shutdownTracer, err := tracing.InstallOpenTelemetryTracer(
		&tracing.Config{
			OtelEndpoint: cfg.Tracing.OtelEndpoint,
			OrgID:        cfg.Tracing.OrgID,
		},
		logger,
		"nodemanager",
		versionString(),
	)
	if err != nil {
		logger.Error("error initializing tracer", "err", err)
		os.Exit(1)
	}
	defer shutdownTracer()

	l, err := lpk.New(*cfg, logger)
	if err != nil {
		logger.Error("failed to create Lpk", "err", err)
		os.Exit(1)
	}

	if err := l.Run(context.Background(), username); err != nil {
		logger.Error("error running LPK", "err", err)
		os.Exit(1)
	}
}

func loadConfig() (*lpk.Config, error) {
	const (
		configFileOption = "config.file"
		usernameOption   = "username"
	)

	var configFile string

	args := os.Args[1:]
	config := &lpk.Config{}

	// first get the config file
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&configFile, configFileOption, "/usr/local/etc/lpk.yaml", "")
	fs.StringVar(&username, usernameOption, "", "")

	config.Tracing.RegisterFlagsAndApplyDefaults("tracing", fs)

	// Try to find -config.file & -config.expand-env flags. As Parsing stops on the first error, eg. unknown flag,
	// we simply try remaining parameters until we find config flag, or there are no params left.
	// (ContinueOnError just means that flag.Parse doesn't call panic or os.Exit, but it returns error, which we ignore)
	for len(args) > 0 {
		_ = fs.Parse(args)
		args = args[1:]
	}

	if len(fs.Args()) == 0 {
		return nil, fmt.Errorf("must pass username argument")
	}

	username = fs.Arg(0)

	// load config defaults and register flags
	config.RegisterFlagsAndApplyDefaults("", flag.CommandLine)

	// overlay with config file if provided
	if configFile != "" {
		buff, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read configFile %s: %w", configFile, err)
		}

		err = yaml.UnmarshalStrict(buff, config)
		if err != nil {
			return nil, fmt.Errorf("failed to parse configFile %s: %w", configFile, err)
		}
	}

	// overlay with cli
	flagext.IgnoredFlag(flag.CommandLine, configFileOption, "Configuration file to load")
	flag.Parse()

	return config, nil
}
