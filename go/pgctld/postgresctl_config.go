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

// PostgresCtlConfig holds all PostgreSQL control configuration parameters
// It contains a PostgresServerConfig for all PostgreSQL-specific settings
// plus additional connection parameters for control operations
type PostgresCtlConfig struct {
	Host     string
	Port     int
	User     string
	Database string
	Password string
	Timeout  int
}

// NewPostgresCtlConfig creates a PostgresCtlConfig with the given parameters
func NewPostgresCtlConfig(host string, port int, user, database, password string, timeout int) *PostgresCtlConfig {
	return &PostgresCtlConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Database: database,
		Password: password,
		Timeout:  timeout,
	}
}
