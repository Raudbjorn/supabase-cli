package sandbox

import (
	"fmt"
	"sort"
)

// serviceMapping maps user-facing service names to process-compose process names.
// Only long-running services are included; one-shot processes (postgres-init, gotrue-migrate)
// are excluded since they don't make sense for restart/stop/start operations.
var serviceMapping = map[string]string{
	"db":   "postgres",
	"auth": "gotrue",
	"rest": "postgrest",
	"api":  "proxy",
}

// ResolveProcessName maps a user-facing service name to the corresponding
// process-compose process name. Returns an error if the name is not valid.
func ResolveProcessName(name string) (string, error) {
	pcName, ok := serviceMapping[name]
	if !ok {
		return "", fmt.Errorf("unknown service %q; valid names: %v", name, ValidServiceNames())
	}
	return pcName, nil
}

// ValidServiceNames returns a sorted list of user-facing service names.
// Used for cobra ValidArgs and help text.
func ValidServiceNames() []string {
	names := make([]string, 0, len(serviceMapping))
	for name := range serviceMapping {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
