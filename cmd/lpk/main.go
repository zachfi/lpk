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
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/flagext"
	"github.com/xaque208/lpk/pkg/lpk"
	"github.com/xaque208/znet/pkg/util"
	yaml "gopkg.in/yaml.v2"
)

var (
	username string
)

func main() {
	logger := util.NewLogger()

	cfg, err := loadConfig()
	if err != nil {
		_ = level.Error(logger).Log("msg", "failed to load config file", "err", err)
		os.Exit(1)
	}

	l, err := lpk.New(*cfg)
	if err != nil {
		_ = level.Error(logger).Log("msg", "failed to create Lpk", "err", err)
		os.Exit(1)
	}

	if err := l.Run(username); err != nil {
		_ = level.Error(logger).Log("msg", "error running Lpk", "err", err)
		os.Exit(1)
	}

}

func loadConfig() (*lpk.Config, error) {
	const (
		configFileOption = "config.file"
		usernameOption   = "username"
	)

	var (
		configFile string
	)

	args := os.Args[1:]
	config := &lpk.Config{}

	// first get the config file
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&configFile, configFileOption, "/usr/local/etc/lpk.yaml", "")
	fs.StringVar(&username, usernameOption, "", "")

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
