package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DockerAuthConfig represents a single registry auth entry from Docker's config.json.
type DockerAuthConfig struct {
	Auth          string `json:"auth"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Email         string `json:"email,omitempty"`
	ServerAddress string `json:"serveraddress,omitempty"`
}

// dockerConfigFile represents the structure of ~/.docker/config.json.
type dockerConfigFile struct {
	Auths map[string]DockerAuthConfig `json:"auths"`
}

// LoadDockerAuthConfig loads the Docker config file and returns the auth for a specific registry.
// Replaces github.com/docker/cli/cli/config.LoadDefaultConfigFile.
// This is a simplified implementation that handles the basic "auths" field.
// Users with credential helpers (credsStore/credHelpers) should set
// INTERNAL_IMAGE_REGISTRY to a public registry or use the default public.ecr.aws.
func LoadDockerAuthConfig(registry string) (DockerAuthConfig, error) {
	configPath := dockerConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DockerAuthConfig{}, nil
		}
		return DockerAuthConfig{}, fmt.Errorf("failed to read docker config: %w", err)
	}

	var config dockerConfigFile
	if err := json.Unmarshal(data, &config); err != nil {
		return DockerAuthConfig{}, fmt.Errorf("failed to parse docker config: %w", err)
	}

	if auth, ok := config.Auths[registry]; ok {
		return auth, nil
	}

	return DockerAuthConfig{}, nil
}

// dockerConfigPath returns the path to Docker's config.json.
func dockerConfigPath() string {
	if configDir := os.Getenv("DOCKER_CONFIG"); configDir != "" {
		return filepath.Join(configDir, "config.json")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".docker", "config.json")
}
