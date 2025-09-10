// Copyright 2025 The Multigres Authors.
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

package pgctld

import (
	"fmt"
	"os"
	"path"
	"strings"
	"text/template"

	"github.com/spf13/pflag"

	"github.com/multigres/multigres/config"
	"github.com/multigres/multigres/go/servenv"
)

// This file handles the creation of PostgresServerConfig objects for the default
// file structure. These paths are used by the pgctld commands.

var poolerDir string

var FlagBinaries = []string{"pgctld", "multipooler"}

func init() {
	for _, cmd := range FlagBinaries {
		servenv.OnParseFor(cmd, registerPostgresConfigFlags)
	}
}

func registerPostgresConfigFlags(fs *pflag.FlagSet) {
	fs.StringVar(&poolerDir, "pooler-dir", poolerDir, "The directory to multipooler data")
}

// GetPoolerDir returns the configured pooler directory
func GetPoolerDir() string {
	return poolerDir
}

// SetPoolerDirForTest sets the pooler directory for testing and returns a cleanup function
func SetPoolerDirForTest(testDir string) func() {
	originalPoolerDir := poolerDir
	poolerDir = testDir
	return func() { poolerDir = originalPoolerDir }
}

// GeneratePostgresServerConfig generates a new PostgreSQL server configuration
// and writes it to disk using the embedded template, then reads it back.
// poolerId is used for the cluster name and path generation.
// port is the port for the PostgreSQL server.
func GeneratePostgresServerConfig(poolerId string, port int) (*PostgresServerConfig, error) {
	if poolerDir == "" {
		return nil, fmt.Errorf("--pooler-dir needs to be set to generate postgres server config")
	}
	configPath := PostgresConfigFile()
	baseDir := PostgresBaseDir()

	// Create minimal config for template generation
	cnf := &PostgresServerConfig{}
	cnf.Path = configPath
	cnf.DataDir = baseDir
	cnf.HbaFile = path.Join(baseDir, "pg_hba.conf")
	cnf.IdentFile = path.Join(baseDir, "pg_ident.conf")
	cnf.Port = port
	cnf.ListenAddresses = "localhost"
	cnf.UnixSocketDirectories = "/tmp"
	cnf.ClusterName = poolerId

	// Generate config file from template
	if err := cnf.generateConfigFile(); err != nil {
		return nil, err
	}

	// Read the generated config back from disk to get all template values
	return ReadPostgresServerConfig(cnf, 0)
}

// LoadOrCreatePostgresServerConfig loads an existing PostgreSQL server configuration
// from disk, or generates and writes a new one if it doesn't exist.
// poolerId is used for the cluster name and path generation.
// port is the port for the PostgreSQL server.
func LoadOrCreatePostgresServerConfig(poolerId string, port int) (*PostgresServerConfig, error) {
	configPath := PostgresConfigFile()

	// Check if config file already exists
	if _, err := os.Stat(configPath); err == nil {
		// Config file exists, read it
		cnf := &PostgresServerConfig{Path: configPath}
		return ReadPostgresServerConfig(cnf, 0)
	}

	// Config file doesn't exist, generate and write it
	return GeneratePostgresServerConfig(poolerId, port)
}

// generateConfigFile creates the postgresql.conf file using the embedded template
func (cnf *PostgresServerConfig) generateConfigFile() error {
	// Ensure directory exists
	if err := os.MkdirAll(path.Dir(cnf.Path), 0755); err != nil {
		return err
	}

	// Generate config content from template
	content, err := cnf.MakePostgresConf(config.PostgresConfigDefaultTmpl)
	if err != nil {
		return err
	}

	// Write to file
	return os.WriteFile(cnf.Path, []byte(content), 0644)
}

// PostgresBaseDir returns the default location of the postgresql.conf file.
func PostgresBaseDir() string {
	return path.Join(poolerDir, "pg")
}

// PostgresConfigFile returns the default location of the postgresql.conf file.
func PostgresConfigFile() string {
	return path.Join(PostgresBaseDir(), "postgresql.conf")
}

// MakePostgresConf will substitute values in the template
func (cnf *PostgresServerConfig) MakePostgresConf(templateContent string) (string, error) {
	pgTemplate, err := template.New("").Parse(templateContent)
	if err != nil {
		return "", err
	}
	var configData strings.Builder
	err = pgTemplate.Execute(&configData, cnf)
	if err != nil {
		return "", err
	}
	return configData.String(), nil
}
