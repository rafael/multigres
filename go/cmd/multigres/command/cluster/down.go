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

package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/filters"
	"github.com/moby/moby/client"
	"github.com/multigres/multigres/go/servenv"

	"github.com/spf13/cobra"
)

// stopMultigresContainers stops all containers with the multigres project label
func stopMultigresContainers(clean bool) error {
	// Create Docker client
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	ctx := context.Background()

	// Find all containers with the multigres project label
	args := filters.NewArgs()
	args.Add("label", "com.docker.compose.project=multigres")
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		Filters: args,
	})
	if err != nil {
		return fmt.Errorf("failed to list multigres containers: %w", err)
	}

	if len(containers) == 0 {
		fmt.Println("No multigres containers are currently running")
		return nil
	}

	fmt.Printf("Found %d multigres container(s) to stop\n", len(containers))

	// Stop the containers
	for _, cont := range containers {
		containerName := strings.TrimPrefix(cont.Names[0], "/")
		fmt.Printf("Stopping container: %s\n", containerName)

		timeout := 30
		if err := cli.ContainerStop(ctx, cont.ID, container.StopOptions{
			Timeout: &timeout,
		}); err != nil {
			fmt.Printf("Warning: failed to stop container %s: %v\n", containerName, err)
		}
	}

	// If clean flag is set, also remove containers and clean up resources
	if clean {
		fmt.Println("Cleaning up containers and resources...")

		// Remove stopped containers
		for _, cont := range containers {
			containerName := strings.TrimPrefix(cont.Names[0], "/")
			if err := cli.ContainerRemove(ctx, cont.ID, container.RemoveOptions{}); err != nil {
				fmt.Printf("Warning: failed to remove container %s: %v\n", containerName, err)
			}
		}

		// Remove the multigres network if it exists
		networkName := "multigres-stack"
		fmt.Printf("Removing network: %s\n", networkName)
		if err := cli.NetworkRemove(ctx, networkName); err != nil {
			fmt.Printf("Warning: failed to remove network %s: %v\n", networkName, err)
		}

		// Optionally remove volumes (commented out for safety - user data)
		// fmt.Println("Removing named volumes...")
		// if err := cli.VolumeRemove(ctx, "multigres-etcd-data", false); err != nil {
		//     fmt.Printf("Warning: failed to remove volume: %v\n", err)
		// }

		fmt.Println("Clean up completed (volumes preserved)")
	}

	return nil
}

// runDown handles the cluster down command
func runDown(cmd *cobra.Command, args []string) error {
	servenv.FireRunHooks()
	fmt.Println("Stopping Multigres cluster...")

	// Get the clean flag
	clean, err := cmd.Flags().GetBool("clean")
	if err != nil {
		return fmt.Errorf("failed to get clean flag: %w", err)
	}

	if clean {
		fmt.Println("Clean mode: will remove containers and networks")
	}

	// Get config paths from flags (for future use if needed)
	configPaths, err := cmd.Flags().GetStringSlice("config-path")
	if err != nil {
		return fmt.Errorf("failed to get config-path flag: %w", err)
	}
	if len(configPaths) == 0 {
		configPaths = []string{"."}
	}

	// Try to load configuration for context, but don't fail if it's not found
	config, configFile, err := LoadConfig(configPaths)
	if err == nil {
		fmt.Printf("Using configuration from: %s\n", configFile)
		fmt.Printf("Stopping cluster with etcd at: %s\n", config.Topology.EtcdDefaultAddress)
	} else {
		fmt.Println("No configuration found, stopping all multigres containers")
	}

	// Stop multigres containers
	if err := stopMultigresContainers(clean); err != nil {
		return fmt.Errorf("failed to stop containers: %w", err)
	}

	fmt.Println("Multigres cluster stopped successfully!")
	return nil
}

var DownCommand = &cobra.Command{
	Use:   "down",
	Short: "Stop local cluster",
	Long:  "Stop the local Multigres cluster. Use --clean to fully tear down all resources.",
	RunE:  runDown,
}

func init() {
	DownCommand.Flags().Bool("clean", false, "Fully tear down all cluster resources")
	// config-path is provided by viperutil via root command
}
