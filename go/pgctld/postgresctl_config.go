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

import "fmt"

// PostgresCtlConfig holds all PostgreSQL control configuration parameters
// It contains a PostgresServerConfig for all PostgreSQL-specific settings
// plus additional connection parameters for control operations
type PostgresCtlConfig struct {
	PostgresConfig *PostgresServerConfig
	Host           string
	User           string
	Database       string
	Password       string
	Timeout        int
}

// Accessor methods for PostgreSQL configuration
func (c *PostgresCtlConfig) Port() int {
	if c.PostgresConfig == nil {
		panic("PostgresConfig is nil - config not properly initialized")
	}
	return c.PostgresConfig.Port
}

func (c *PostgresCtlConfig) DataDir() string {
	if c.PostgresConfig == nil {
		panic("PostgresConfig is nil - config not properly initialized")
	}
	return c.PostgresConfig.DataDir
}

func (c *PostgresCtlConfig) SocketDir() string {
	if c.PostgresConfig == nil {
		panic("PostgresConfig is nil - config not properly initialized")
	}
	return c.PostgresConfig.UnixSocketDirectories
}

func (c *PostgresCtlConfig) ConfigFile() string {
	if c.PostgresConfig == nil {
		panic("PostgresConfig is nil - config not properly initialized")
	}
	return c.PostgresConfig.Path
}

func (c *PostgresCtlConfig) ListenAddresses() string {
	if c.PostgresConfig == nil {
		panic("PostgresConfig is nil - config not properly initialized")
	}
	return c.PostgresConfig.ListenAddresses
}

func (c *PostgresCtlConfig) ClusterName() string {
	if c.PostgresConfig == nil {
		panic("PostgresConfig is nil - config not properly initialized")
	}
	return c.PostgresConfig.ClusterName
}

func (c *PostgresCtlConfig) HbaFile() string {
	if c.PostgresConfig == nil {
		panic("PostgresConfig is nil - config not properly initialized")
	}
	return c.PostgresConfig.HbaFile
}

func (c *PostgresCtlConfig) IdentFile() string {
	if c.PostgresConfig == nil {
		panic("PostgresConfig is nil - config not properly initialized")
	}
	return c.PostgresConfig.IdentFile
}

func (c *PostgresCtlConfig) MaxConnections() int {
	if c.PostgresConfig == nil {
		panic("PostgresConfig is nil - config not properly initialized")
	}
	return c.PostgresConfig.MaxConnections
}

// NewPostgresCtlConfig creates a PostgresCtlConfig with the given parameters
func NewPostgresCtlConfig(pgConfig *PostgresServerConfig, host, user, database, password string, timeout int) *PostgresCtlConfig {
	return &PostgresCtlConfig{
		PostgresConfig: pgConfig,
		Host:           host,
		User:           user,
		Database:       database,
		Password:       password,
		Timeout:        timeout,
	}
}

// NewPostgresCtlConfigFromDefaults creates a PostgresCtlConfig with default values
// This function loads or creates a PostgreSQL server configuration
func NewPostgresCtlConfigFromDefaults(pgPort int, pgHost, pgUser, pgDatabase, pgPassword string, timeout int) (*PostgresCtlConfig, error) {
	// Load or create PostgreSQL server config
	pgConfig, err := LoadOrCreatePostgresServerConfig("default", pgPort)
	if err != nil {
		return nil, fmt.Errorf("failed to load postgres config: %w", err)
	}

	return &PostgresCtlConfig{
		PostgresConfig: pgConfig,
		Host:           pgHost,
		User:           pgUser,
		Database:       pgDatabase,
		Password:       pgPassword,
		Timeout:        timeout,
	}, nil
}
