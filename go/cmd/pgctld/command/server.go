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
	"context"
	"fmt"
	"log/slog"

	"github.com/multigres/multigres/go/pgctld"
	"github.com/multigres/multigres/go/servenv"

	"github.com/spf13/cobra"

	pb "github.com/multigres/multigres/go/pb/pgctldservice"
)

func init() {
	Root.AddCommand(ServerCmd)
}

var ServerCmd = &cobra.Command{
	Use:     "server",
	Short:   "Run pgctld as a gRPC server daemon",
	Long:    `Run pgctld as a background gRPC server daemon to handle PostgreSQL management requests.`,
	RunE:    runServer,
	Args:    cobra.NoArgs,
	PreRunE: servenv.CobraPreRunE,
}

func runServer(cmd *cobra.Command, args []string) error {
	servenv.Init()

	// Get the configured logger
	logger := servenv.GetLogger()

	// Create and register our service
	pgctldService := &PgCtldService{
		logger: logger,
	}

	servenv.OnRun(func() {
		logger.Info("pgctld server starting up",
			"grpc_port", servenv.GRPCPort(),
		)

		// Register gRPC service with the global GRPCServer
		if servenv.GRPCCheckServiceMap("pgctld") {
			pb.RegisterPgCtldServer(servenv.GRPCServer, pgctldService)
		}
	})

	servenv.OnClose(func() {
		logger.Info("pgctld server shutting down")
		// TODO: add closing hooks
	})

	servenv.RunDefault()

	return nil
}

// PgCtldService implements the pgctld gRPC service
type PgCtldService struct {
	pb.UnimplementedPgCtldServer
	logger *slog.Logger
}

func (s *PgCtldService) Start(ctx context.Context, req *pb.StartRequest) (*pb.StartResponse, error) {
	s.logger.Info("gRPC Start request", "port", req.Port)

	// Create config from request parameters
	config, err := NewPostgresConfigFromStartRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create config: %w", err)
	}

	// Use the shared start function with detailed result
	result, err := StartPostgreSQLWithResult(config)
	if err != nil {
		return nil, fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	return &pb.StartResponse{
		Pid:     int32(result.PID),
		Message: result.Message,
	}, nil
}

func (s *PgCtldService) Stop(ctx context.Context, req *pb.StopRequest) (*pb.StopResponse, error) {
	s.logger.Info("gRPC Stop request", "mode", req.Mode)

	// Create config from request parameters
	config, err := NewPostgresConfigFromStopRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create config: %w", err)
	}

	// Use the shared stop function with detailed result
	result, err := StopPostgreSQLWithResult(config, req.Mode)
	if err != nil {
		return nil, fmt.Errorf("failed to stop PostgreSQL: %w", err)
	}

	return &pb.StopResponse{
		Message: result.Message,
	}, nil
}

func (s *PgCtldService) Restart(ctx context.Context, req *pb.RestartRequest) (*pb.RestartResponse, error) {
	s.logger.Info("gRPC Restart request", "mode", req.Mode, "port", req.Port)

	// Determine the port to use
	port := pgPort
	if req.Port > 0 {
		port = int(req.Port)
	}

	// Load or create PostgreSQL server config
	pgConfig, err := pgctld.LoadOrCreatePostgresServerConfig("default", port)
	if err != nil {
		return nil, fmt.Errorf("failed to load postgres config: %w", err)
	}

	timeout := timeout
	if req.Timeout > 0 {
		timeout = int(req.Timeout)
	}

	config := pgctld.NewPostgresCtlConfig(pgConfig, pgHost, pgUser, pgDatabase, pgPassword, timeout)

	// Use the shared restart function with detailed result
	result, err := RestartPostgreSQLWithResult(config, req.Mode)
	if err != nil {
		return nil, fmt.Errorf("failed to restart PostgreSQL: %w", err)
	}

	return &pb.RestartResponse{
		Pid:     int32(result.PID),
		Message: result.Message,
	}, nil
}

func (s *PgCtldService) ReloadConfig(ctx context.Context, req *pb.ReloadConfigRequest) (*pb.ReloadConfigResponse, error) {
	s.logger.Info("gRPC ReloadConfig request")

	// Create config from request parameters
	config, err := NewPostgresCtlConfigFromDefaults()
	if err != nil {
		return nil, fmt.Errorf("failed to create config: %w", err)
	}

	// Use the shared reload function with detailed result
	result, err := ReloadPostgreSQLConfigWithResult(config)
	if err != nil {
		return nil, fmt.Errorf("failed to reload PostgreSQL configuration: %w", err)
	}

	return &pb.ReloadConfigResponse{
		Message: result.Message,
	}, nil
}

func (s *PgCtldService) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	s.logger.Debug("gRPC Status request")

	// Create config from request parameters
	config, err := NewPostgresConfigFromStatusRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create config: %w", err)
	}

	// Use the shared status function with detailed result
	result, err := GetStatusWithResult(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	// Convert status string to protobuf enum
	var status pb.ServerStatus
	switch result.Status {
	case "NOT_INITIALIZED":
		status = pb.ServerStatus_NOT_INITIALIZED
	case "STOPPED":
		status = pb.ServerStatus_STOPPED
	case "RUNNING":
		status = pb.ServerStatus_RUNNING
	default:
		status = pb.ServerStatus_STOPPED
	}

	return &pb.StatusResponse{
		Status:        status,
		Pid:           int32(result.PID),
		Version:       result.Version,
		UptimeSeconds: result.UptimeSeconds,
		DataDir:       result.DataDir,
		Port:          int32(result.Port),
		Host:          result.Host,
		Ready:         result.Ready,
		Message:       result.Message,
	}, nil
}

func (s *PgCtldService) Version(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	s.logger.Debug("gRPC Version request")

	// Determine the port to use
	port := pgPort
	if req.Port > 0 {
		port = int(req.Port)
	}

	// Load or create PostgreSQL server config
	pgConfig, err := pgctld.LoadOrCreatePostgresServerConfig("default", port)
	if err != nil {
		return nil, fmt.Errorf("failed to load postgres config: %w", err)
	}

	// Create config with overrides from request
	config := &pgctld.PostgresCtlConfig{
		PostgresConfig: pgConfig,
		Host:           pgHost,
		User:           pgUser,
		Database:       pgDatabase,
		Password:       pgPassword,
		Timeout:        timeout,
	}

	// Override with request parameters if provided
	if req.Host != "" {
		config.Host = req.Host
	}
	if req.Database != "" {
		config.Database = req.Database
	}
	if req.User != "" {
		config.User = req.User
	}

	// Use the shared version function with detailed result
	result, err := GetVersionWithResult(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}

	return &pb.VersionResponse{
		Version: result.Version,
		Message: result.Message,
	}, nil
}

func (s *PgCtldService) InitDataDir(ctx context.Context, req *pb.InitDataDirRequest) (*pb.InitDataDirResponse, error) {
	s.logger.Info("gRPC InitDataDir request")

	// Create config from request parameters
	config, err := pgctld.LoadOrCreatePostgresServerConfig("default", pgPort)
	if err != nil {
		return nil, fmt.Errorf("failed to load or create PostgresServerConfig: %w", err)
	}

	// Use the shared init function with detailed result
	result, err := InitDataDirWithResult(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize data directory: %w", err)
	}

	return &pb.InitDataDirResponse{
		Message: result.Message,
	}, nil
}
