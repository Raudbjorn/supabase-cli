package utils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/docker/docker/client"
	"go.opentelemetry.io/otel"
)

// This file holds the only two functions in this package allowed to diverge
// from upstream's apps/cli-go/internal/utils/docker.go: both exist solely to
// drop the docker/cli dependency (see FORK_MAINTENANCE.md). Everything else
// in docker.go matches upstream and should keep merging clean; if a future
// merge conflicts inside docker.go outside of these two functions, something
// has drifted and needs investigation rather than blindly keeping ours.

func NewDocker() *client.Client {
	// Silence otel errors as users don't care about docker metrics
	// 2024/08/12 23:11:12 1 errors occurred detecting resource:
	// 	* conflicting Schema URL: https://opentelemetry.io/schemas/1.21.0
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(cause error) {}))
	// Use docker/docker/client directly instead of going through docker/cli.
	// This eliminates the heavy docker/cli dependency tree.
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalln("Failed to create Docker client:", err)
	}
	return cli
}

func loadRegistryAuth(registry string) string {
	// Load auth config directly from ~/.docker/config.json instead of using
	// docker/cli's config loader. This eliminates the docker/cli dependency.
	auth, err := LoadDockerAuthConfig(registry)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to load registry credentials:", err)
		return ""
	}
	if auth.Auth == "" && auth.Username == "" {
		return ""
	}
	encoded, err := json.Marshal(auth)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to serialise auth config:", err)
		return ""
	}
	return base64.URLEncoding.EncodeToString(encoded)
}
