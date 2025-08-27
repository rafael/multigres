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

package init

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestInitCommand(t *testing.T) {
	tests := []struct {
		name           string
		setupDirs      func(*testing.T) ([]string, func()) // returns config paths and cleanup
		expectError    bool
		errorContains  string
		outputContains []string
	}{
		{
			name: "successful init with current directory",
			setupDirs: func(t *testing.T) ([]string, func()) {
				tempDir, err := os.MkdirTemp("", "multigres_init_test")
				require.NoError(t, err)
				return []string{tempDir}, func() { os.RemoveAll(tempDir) }
			},
			expectError:    false,
			outputContains: []string{"Initializing Multigres cluster configuration", "successfully"},
		},
		{
			name: "error with non-existent config path",
			setupDirs: func(t *testing.T) ([]string, func()) {
				return []string{"/nonexistent/path/that/should/not/exist"}, func() {}
			},
			expectError:   true,
			errorContains: "config path does not exist",
		},
		{
			name: "error with file instead of directory",
			setupDirs: func(t *testing.T) ([]string, func()) {
				tempFile, err := os.CreateTemp("", "multigres_init_test_file")
				require.NoError(t, err)
				tempFile.Close()
				return []string{tempFile.Name()}, func() { os.Remove(tempFile.Name()) }
			},
			expectError:   true,
			errorContains: "config path is not a directory",
		},
		{
			name: "successful init with multiple valid paths",
			setupDirs: func(t *testing.T) ([]string, func()) {
				tempDir1, err := os.MkdirTemp("", "multigres_init_test1")
				require.NoError(t, err)
				tempDir2, err := os.MkdirTemp("", "multigres_init_test2")
				require.NoError(t, err)
				return []string{tempDir1, tempDir2}, func() {
					os.RemoveAll(tempDir1)
					os.RemoveAll(tempDir2)
				}
			},
			expectError:    false,
			outputContains: []string{"Initializing Multigres cluster configuration", "successfully"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test directories
			configPaths, cleanup := tt.setupDirs(t)
			defer cleanup()

			// Create a copy of the command for testing
			cmd := &cobra.Command{
				Use:  "init",
				RunE: runInit,
			}

			// Add the config-path flag
			cmd.Flags().StringSlice("config-path", configPaths, "test config paths")

			// Capture stdout (the actual output from fmt.Println calls)
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Capture stderr
			var stderr bytes.Buffer
			cmd.SetErr(&stderr)

			// Execute command
			err := cmd.Execute()

			// Close writer and restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read captured output
			var output bytes.Buffer
			_, err2 := output.ReadFrom(r)
			require.NoError(t, err2)

			// Check results
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				outputStr := output.String()
				for _, expectedOutput := range tt.outputContains {
					assert.Contains(t, outputStr, expectedOutput)
				}
			}
		})
	}
}

func TestInitCommand_NoConfigPaths(t *testing.T) {
	// Create a command with no config-path flag set
	cmd := &cobra.Command{
		Use:  "init",
		RunE: runInit,
	}

	// Add empty config-path flag
	cmd.Flags().StringSlice("config-path", []string{}, "test config paths")

	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no config paths specified")
}

func TestInitCommand_ConfigFileCreation(t *testing.T) {
	// Setup test directory
	tempDir, err := os.MkdirTemp("", "multigres_init_config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create command
	cmd := &cobra.Command{
		Use:  "init",
		RunE: runInit,
	}
	cmd.Flags().StringSlice("config-path", []string{tempDir}, "test config paths")

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute command
	err = cmd.Execute()

	// Restore stdout and read output
	w.Close()
	os.Stdout = oldStdout
	var output bytes.Buffer
	_, err2 := output.ReadFrom(r)
	require.NoError(t, err2)

	// Command should succeed
	require.NoError(t, err)

	// Check output contains expected messages
	outputStr := output.String()
	assert.Contains(t, outputStr, "Initializing Multigres cluster configuration")
	assert.Contains(t, outputStr, "Created configuration file")
	assert.Contains(t, outputStr, "successfully")

	// Check config file was created
	configFile := filepath.Join(tempDir, "multigres.yaml")
	_, err = os.Stat(configFile)
	require.NoError(t, err, "Config file should exist")

	// Read and validate config content
	configData, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var config MultigressConfig
	err = yaml.Unmarshal(configData, &config)
	require.NoError(t, err)

	// Verify config values
	assert.Equal(t, "local", config.Provisioner)
	assert.Equal(t, "etcd", config.Topology.Implementation)
	assert.Equal(t, "/multigres/global", config.Topology.GlobalRootPath)
	assert.Equal(t, "zone1", config.Topology.DefaultCellName)
	assert.Equal(t, "/multigres/zone1", config.Topology.DefaultCellRootPath)
}

func TestInitCommand_ConfigFileAlreadyExists(t *testing.T) {
	// Setup test directory
	tempDir, err := os.MkdirTemp("", "multigres_init_exists_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create existing config file
	existingConfig := filepath.Join(tempDir, "multigres.yaml")
	err = os.WriteFile(existingConfig, []byte("existing: config"), 0644)
	require.NoError(t, err)

	// Create command
	cmd := &cobra.Command{
		Use:  "init",
		RunE: runInit,
	}
	cmd.Flags().StringSlice("config-path", []string{tempDir}, "test config paths")

	// Execute command
	err = cmd.Execute()

	// Should fail with appropriate error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file already exists")
	assert.Contains(t, err.Error(), existingConfig)
}
