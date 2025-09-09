/*
Copyright 2025 The Multigres Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/multigres/multigres/go/cmd/pgctld/testutil"
	"github.com/multigres/multigres/go/pgctld"
)

func TestStopPostgreSQLWithResult(t *testing.T) {
	tests := []struct {
		name           string
		setupPoolerDir func(string) string
		setupBinaries  bool
		mode           string
		config         func(*pgctld.PostgresCtlConfig) *pgctld.PostgresCtlConfig
		expectError    bool
		errorContains  string
		expectedResult func(*StopResult)
	}{
		{
			name: "successful stop with fast mode",
			setupPoolerDir: func(baseDir string) string {
				poolerDir := baseDir
				pgctld.SetPoolerDirForTest(poolerDir)
				pgConfig, _ := pgctld.GeneratePostgresServerConfig("test", 5432)
				testutil.CreateDataDir(t, pgConfig.DataDir, true)
				testutil.CreatePIDFile(t, pgConfig.DataDir, 12345)
				return poolerDir
			},
			setupBinaries: true,
			mode:          "fast",
			config: func(config *pgctld.PostgresCtlConfig) *pgctld.PostgresCtlConfig {
				return config
			},
			expectError: false,
			expectedResult: func(result *StopResult) {
				assert.True(t, result.WasRunning)
				assert.Equal(t, "PostgreSQL server stopped successfully", result.Message)
			},
		},
		{
			name: "successful stop with smart mode",
			setupPoolerDir: func(baseDir string) string {
				poolerDir := baseDir
				pgctld.SetPoolerDirForTest(poolerDir)
				pgConfig, _ := pgctld.GeneratePostgresServerConfig("test", 5432)
				testutil.CreateDataDir(t, pgConfig.DataDir, true)
				testutil.CreatePIDFile(t, pgConfig.DataDir, 12345)
				return poolerDir
			},
			setupBinaries: true,
			mode:          "smart",
			config: func(config *pgctld.PostgresCtlConfig) *pgctld.PostgresCtlConfig {
				return config
			},
			expectError: false,
			expectedResult: func(result *StopResult) {
				assert.True(t, result.WasRunning)
				assert.Equal(t, "PostgreSQL server stopped successfully", result.Message)
			},
		},
		{
			name: "successful stop with immediate mode",
			setupPoolerDir: func(baseDir string) string {
				poolerDir := baseDir
				pgctld.SetPoolerDirForTest(poolerDir)
				pgConfig, _ := pgctld.GeneratePostgresServerConfig("test", 5432)
				testutil.CreateDataDir(t, pgConfig.DataDir, true)
				testutil.CreatePIDFile(t, pgConfig.DataDir, 12345)
				return poolerDir
			},
			setupBinaries: true,
			mode:          "immediate",
			config: func(config *pgctld.PostgresCtlConfig) *pgctld.PostgresCtlConfig {
				return config
			},
			expectError: false,
			expectedResult: func(result *StopResult) {
				assert.True(t, result.WasRunning)
				assert.Equal(t, "PostgreSQL server stopped successfully", result.Message)
			},
		},
		{
			name: "stop when PostgreSQL is not running",
			setupPoolerDir: func(baseDir string) string {
				poolerDir := baseDir
				pgctld.SetPoolerDirForTest(poolerDir)
				pgConfig, _ := pgctld.GeneratePostgresServerConfig("test", 5432)
				testutil.CreateDataDir(t, pgConfig.DataDir, true)
				// Don't create PID file (not running)
				return poolerDir
			},
			setupBinaries: false,
			mode:          "fast",
			config: func(config *pgctld.PostgresCtlConfig) *pgctld.PostgresCtlConfig {
				return config
			},
			expectError: false,
			expectedResult: func(result *StopResult) {
				assert.False(t, result.WasRunning)
				assert.Equal(t, "PostgreSQL is not running", result.Message)
			},
		},
		{
			name:           "error when data-dir is empty",
			setupPoolerDir: func(baseDir string) string { return "" },
			mode:           "fast",
			config: func(config *pgctld.PostgresCtlConfig) *pgctld.PostgresCtlConfig {
				// Create a new config with empty DataDir
				emptyPgConfig, _ := pgctld.GeneratePostgresServerConfig("test", 5432)
				emptyPgConfig.DataDir = ""
				return pgctld.NewPostgresCtlConfig(emptyPgConfig, "localhost", "postgres", "postgres", "", 30)
			},
			expectError:   true,
			errorContains: "pg-data-dir is required",
		},
		{
			name: "default mode when empty string provided",
			setupPoolerDir: func(baseDir string) string {
				poolerDir := baseDir
				pgctld.SetPoolerDirForTest(poolerDir)
				pgConfig, _ := pgctld.GeneratePostgresServerConfig("test", 5432)
				testutil.CreateDataDir(t, pgConfig.DataDir, true)
				testutil.CreatePIDFile(t, pgConfig.DataDir, 12345)
				return poolerDir
			},
			setupBinaries: true,
			mode:          "", // Empty mode should default to "fast"
			config: func(config *pgctld.PostgresCtlConfig) *pgctld.PostgresCtlConfig {
				return config
			},
			expectError: false,
			expectedResult: func(result *StopResult) {
				assert.True(t, result.WasRunning)
				assert.Equal(t, "PostgreSQL server stopped successfully", result.Message)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir, cleanup := testutil.TempDir(t, "pgctld_stop_test")
			defer cleanup()

			tt.setupPoolerDir(baseDir)

			if tt.setupBinaries {
				binDir := filepath.Join(baseDir, "bin")
				require.NoError(t, os.MkdirAll(binDir, 0755))
				testutil.CreateMockPostgreSQLBinaries(t, binDir)

				originalPath := os.Getenv("PATH")
				os.Setenv("PATH", binDir+":"+originalPath)
				defer os.Setenv("PATH", originalPath)
			}

			// Create a mock PostgreSQL server config
			pgConfig, err := pgctld.GeneratePostgresServerConfig("test", 5432)
			require.NoError(t, err)

			config := pgctld.NewPostgresCtlConfig(pgConfig, "localhost", "postgres", "postgres", "", 30)
			config = tt.config(config)

			result, err := StopPostgreSQLWithResult(config, tt.mode)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.expectedResult != nil {
					tt.expectedResult(result)
				}
			}
		})
	}
}

func TestRunStop(t *testing.T) {
	tests := []struct {
		name           string
		setupPoolerDir func(string) string
		setupBinaries  bool
		mode           string
		expectError    bool
		errorContains  string
	}{
		{
			name: "successful stop command",
			setupPoolerDir: func(baseDir string) string {
				poolerDir := baseDir
				pgctld.SetPoolerDirForTest(poolerDir)
				pgConfig, _ := pgctld.GeneratePostgresServerConfig("test", 5432)
				testutil.CreateDataDir(t, pgConfig.DataDir, true)
				testutil.CreatePIDFile(t, pgConfig.DataDir, 12345)
				return poolerDir
			},
			setupBinaries: true,
			mode:          "fast",
			expectError:   false,
		},
		{
			name: "stop when not running",
			setupPoolerDir: func(baseDir string) string {
				poolerDir := baseDir
				pgctld.SetPoolerDirForTest(poolerDir)
				pgConfig, _ := pgctld.GeneratePostgresServerConfig("test", 5432)
				testutil.CreateDataDir(t, pgConfig.DataDir, true)
				// Don't create PID file (not running)
				return poolerDir
			},
			setupBinaries: false,
			mode:          "fast",
			expectError:   false,
		},
		{
			name: "stop with smart mode",
			setupPoolerDir: func(baseDir string) string {
				// This is to mock the case where the pooler dir was already set for testing
				poolerDir := baseDir
				cleanup := pgctld.SetPoolerDirForTest(poolerDir)
				defer cleanup()
				pgConfig, _ := pgctld.GeneratePostgresServerConfig("test", 5432)
				testutil.CreateDataDir(t, pgConfig.DataDir, true)
				testutil.CreatePIDFile(t, pgConfig.DataDir, 12345)
				return poolerDir
			},
			setupBinaries: true,
			mode:          "smart",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir, cleanup := testutil.TempDir(t, "pgctld_run_stop_test")
			defer cleanup()

			// Setup cleanup for cobra command execution
			cleanupViper := SetupTestPgCtldCleanup(t)
			defer cleanupViper()

			poolerDir := tt.setupPoolerDir(baseDir)

			if tt.setupBinaries {
				binDir := filepath.Join(baseDir, "bin")
				require.NoError(t, os.MkdirAll(binDir, 0755))
				testutil.CreateMockPostgreSQLBinaries(t, binDir)

				originalPath := os.Getenv("PATH")
				os.Setenv("PATH", binDir+":"+originalPath)
				defer os.Setenv("PATH", originalPath)
			}

			cmd := Root

			// Set up the command arguments
			args := []string{"stop", "--mode", tt.mode}
			if poolerDir != "" {
				args = append(args, "--pooler-dir", poolerDir)
			}
			cmd.SetArgs(args)

			err := cmd.Execute()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStopPostgreSQLWithConfig(t *testing.T) {
	tests := []struct {
		name           string
		setupPoolerDir func(string) string
		setupBinaries  bool
		mode           string
		expectError    bool
	}{
		{
			name: "successful stop via config wrapper",
			setupPoolerDir: func(baseDir string) string {
				// This is to mock the case where the pooler dir was already set for testing
				poolerDir := baseDir
				cleanup := pgctld.SetPoolerDirForTest(poolerDir)
				defer cleanup()
				pgConfig, _ := pgctld.GeneratePostgresServerConfig("test", 5432)
				testutil.CreateDataDir(t, pgConfig.DataDir, true)
				testutil.CreatePIDFile(t, pgConfig.DataDir, 12345)
				return poolerDir
			},
			setupBinaries: true,
			mode:          "fast",
			expectError:   false,
		},
		{
			name: "stop when not running via config wrapper",
			setupPoolerDir: func(baseDir string) string {
				// This is to mock the case where the pooler dir was already set for testing
				poolerDir := baseDir
				cleanup := pgctld.SetPoolerDirForTest(poolerDir)
				defer cleanup()
				pgConfig, _ := pgctld.GeneratePostgresServerConfig("test", 5432)
				testutil.CreateDataDir(t, pgConfig.DataDir, true)
				// Don't create PID file (not running)
				return poolerDir
			},
			setupBinaries: false,
			mode:          "fast",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir, cleanup := testutil.TempDir(t, "pgctld_stop_config_test")
			defer cleanup()

			tt.setupPoolerDir(baseDir)

			if tt.setupBinaries {
				binDir := filepath.Join(baseDir, "bin")
				require.NoError(t, os.MkdirAll(binDir, 0755))
				testutil.CreateMockPostgreSQLBinaries(t, binDir)

				originalPath := os.Getenv("PATH")
				os.Setenv("PATH", binDir+":"+originalPath)
				defer os.Setenv("PATH", originalPath)
			}

			// Create a mock PostgreSQL server config
			pgConfig, err := pgctld.GeneratePostgresServerConfig("test", 5432)
			require.NoError(t, err)

			config := pgctld.NewPostgresCtlConfig(pgConfig, "localhost", "postgres", "postgres", "", 30)

			err = StopPostgreSQLWithConfig(config, tt.mode)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTakeCheckpoint(t *testing.T) {
	tests := []struct {
		name          string
		setupBinaries bool
		config        *pgctld.PostgresCtlConfig
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful checkpoint",
			setupBinaries: true,
			config: func() *pgctld.PostgresCtlConfig {
				pgConfig, _ := pgctld.GeneratePostgresServerConfig("test", 5432)
				pgConfig.DataDir = "/tmp/test"
				return pgctld.NewPostgresCtlConfig(pgConfig, "localhost", "postgres", "postgres", "", 30)
			}(),
			expectError: false,
		},
		{
			name:          "checkpoint with password",
			setupBinaries: true,
			config: &pgctld.PostgresCtlConfig{
				Host:     "localhost",
				User:     "postgres",
				Database: "postgres",
				Password: "secret",
			},
			expectError: false,
		},
		{
			name:          "checkpoint failure - psql command fails",
			setupBinaries: true, // Create failing psql binary
			config: &pgctld.PostgresCtlConfig{
				Host:     "localhost",
				User:     "postgres",
				Database: "postgres",
			},
			expectError:   true,
			errorContains: "checkpoint command failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir, cleanup := testutil.TempDir(t, "pgctld_checkpoint_test")
			defer cleanup()

			if tt.setupBinaries {
				binDir := filepath.Join(baseDir, "bin")
				require.NoError(t, os.MkdirAll(binDir, 0755))

				if tt.name == "checkpoint failure - psql command fails" {
					// Create a psql that always fails
					testutil.MockBinary(t, binDir, "psql", "exit 1")
				} else {
					testutil.CreateMockPostgreSQLBinaries(t, binDir)
				}

				originalPath := os.Getenv("PATH")
				os.Setenv("PATH", binDir+":"+originalPath)
				defer os.Setenv("PATH", originalPath)
			}

			err := takeCheckpoint(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStopResult(t *testing.T) {
	t.Run("StopResult struct creation and validation", func(t *testing.T) {
		// Test StopResult struct
		result := &StopResult{
			WasRunning: true,
			Message:    "Test message",
		}

		assert.True(t, result.WasRunning)
		assert.Equal(t, "Test message", result.Message)

		// Test empty result
		emptyResult := &StopResult{}
		assert.False(t, emptyResult.WasRunning)
		assert.Empty(t, emptyResult.Message)
	})
}
