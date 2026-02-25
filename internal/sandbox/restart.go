package sandbox

// KnownServices maps user-facing service names to process-compose process names.
// Includes both canonical and alias names for user convenience.
var KnownServices = map[string]string{
	"postgres":  "postgres",
	"db":        "postgres",
	"gotrue":    "gotrue",
	"auth":      "gotrue",
	"postgrest": "postgrest",
	"rest":      "postgrest",
	"proxy":     "proxy",
	"api":       "proxy",
}

// RestartableServices returns the list of user-facing service names.
func RestartableServices() []string {
	return []string{"postgres", "auth", "rest", "api"}
}
