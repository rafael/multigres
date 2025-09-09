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

package pgctld

import (
	"strings"
	"testing"
)

func TestNewPostgresServerConfig(t *testing.T) {
	// Set up poolerDir for testing
	poolerDir = "/test/pooler"

	tests := []struct {
		name        string
		poolerId    string
		port        int
		wantPort    int
		wantCluster string
		wantDataDir string
	}{
		{
			name:        "basic config creation",
			poolerId:    "test-pooler-1",
			port:        5432,
			wantPort:    5432,
			wantCluster: "test-pooler-1",
			wantDataDir: "/test/pooler/pg",
		},
		{
			name:        "custom port",
			poolerId:    "pooler-2",
			port:        5433,
			wantPort:    5433,
			wantCluster: "pooler-2",
			wantDataDir: "/test/pooler/pg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := LoadPostgresServerConfig(tt.poolerId, tt.port)
			if err != nil {
				t.Fatalf("LoadPostgresServerConfig() error = %v", err)
			}

			if config.Port != tt.wantPort {
				t.Errorf("Port = %v, want %v", config.Port, tt.wantPort)
			}

			if config.ClusterName != tt.wantCluster {
				t.Errorf("ClusterName = %v, want %v", config.ClusterName, tt.wantCluster)
			}

			if config.DataDir != tt.wantDataDir {
				t.Errorf("DataDirectory = %v, want %v", config.DataDir, tt.wantDataDir)
			}

			if config.ListenAddresses != "localhost" {
				t.Errorf("ListenAddresses = %v, want 'localhost'", config.ListenAddresses)
			}

			if config.UnixSocketDirectories != "/tmp" {
				t.Errorf("UnixSocketDirectories = %v, want '/tmp'", config.UnixSocketDirectories)
			}
		})
	}
}

func TestPostgresBaseDir(t *testing.T) {
	// Set up poolerDir for testing
	poolerDir = "/test/pooler"

	expected := "/test/pooler/pg"
	result := PostgresBaseDir()

	if result != expected {
		t.Errorf("PostgresBaseDir() = %v, want %v", result, expected)
	}
}

func TestPostgresConfigFile(t *testing.T) {
	// Set up poolerDir for testing
	poolerDir = "/test/pooler"

	expected := "/test/pooler/pg/postgresql.conf"
	result := PostgresConfigFile()

	if result != expected {
		t.Errorf("PostgresConfigFile() = %v, want %v", result, expected)
	}
}

func TestMakePostgresConf(t *testing.T) {
	// Set up poolerDir for testing
	poolerDir = "/test/pooler"

	config, err := LoadPostgresServerConfig("test-pooler", 5432)
	if err != nil {
		t.Fatalf("LoadPostgresServerConfig() error = %v", err)
	}

	tests := []struct {
		name     string
		template string
		want     []string // strings that should be present in output
		wantNot  []string // strings that should NOT be present in output
	}{
		{
			name:     "port template",
			template: "port = {{.Port}}",
			want:     []string{"port = 5432"},
		},
		{
			name:     "cluster name template",
			template: "cluster_name = '{{.ClusterName}}'",
			want:     []string{"cluster_name = 'test-pooler'"},
		},
		{
			name:     "data directory template",
			template: "data_directory = '{{.DataDirectory}}'",
			want:     []string{"data_directory = '/test/pooler/pg'"},
		},
		{
			name:     "max connections template",
			template: "max_connections = {{.MaxConnections}}",
			want:     []string{"max_connections = 100"},
		},
		{
			name:     "listen addresses template",
			template: "listen_addresses = '{{.ListenAddresses}}'",
			want:     []string{"listen_addresses = 'localhost'"},
		},
		{
			name:     "unix socket directories template",
			template: "unix_socket_directories = '{{.UnixSocketDirectories}}'",
			want:     []string{"unix_socket_directories = '/tmp'"},
		},
		{
			name: "complex template",
			template: `# PostgreSQL Configuration
port = {{.Port}}
max_connections = {{.MaxConnections}}
listen_addresses = '{{.ListenAddresses}}'
data_directory = '{{.DataDirectory}}'
cluster_name = '{{.ClusterName}}'
unix_socket_directories = '{{.UnixSocketDirectories}}'`,
			want: []string{
				"port = 5432",
				"max_connections = 100",
				"listen_addresses = 'localhost'",
				"data_directory = '/test/pooler/pg'",
				"cluster_name = 'test-pooler'",
				"unix_socket_directories = '/tmp'",
				"# PostgreSQL Configuration",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.MakePostgresConf(tt.template)
			if err != nil {
				t.Fatalf("MakePostgresConf() error = %v", err)
			}

			// Check that wanted strings are present
			for _, want := range tt.want {
				if !strings.Contains(result, want) {
					t.Errorf("MakePostgresConf() result missing expected string: %q\nFull result:\n%s", want, result)
				}
			}

			// Check that unwanted strings are not present
			for _, wantNot := range tt.wantNot {
				if strings.Contains(result, wantNot) {
					t.Errorf("MakePostgresConf() result contains unwanted string: %q\nFull result:\n%s", wantNot, result)
				}
			}
		})
	}
}

func TestMakePostgresConfInvalidTemplate(t *testing.T) {
	// Set up poolerDir for testing
	poolerDir = "/test/pooler"

	config, err := LoadPostgresServerConfig("test-pooler", 5432)
	if err != nil {
		t.Fatalf("LoadPostgresServerConfig() error = %v", err)
	}

	tests := []struct {
		name     string
		template string
	}{
		{
			name:     "invalid template syntax",
			template: "port = {{.Port",
		},
		{
			name:     "unknown field",
			template: "unknown = {{.UnknownField}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.MakePostgresConf(tt.template)
			if err == nil {
				t.Errorf("MakePostgresConf() expected error for invalid template, got nil")
			}
		})
	}
}

func TestGetPoolerDir(t *testing.T) {
	// Set up poolerDir for testing
	originalPoolerDir := poolerDir
	defer func() { poolerDir = originalPoolerDir }()

	testDir := "/test/custom/pooler"
	poolerDir = testDir

	result := GetPoolerDir()
	if result != testDir {
		t.Errorf("GetPoolerDir() = %v, want %v", result, testDir)
	}

	// Test empty case
	poolerDir = ""
	result = GetPoolerDir()
	if result != "" {
		t.Errorf("GetPoolerDir() = %v, want empty string", result)
	}
}
