package build

import (
	"fmt"
)

// mapToEnv converts a map of environment variables to a slice of KEY=VALUE strings
func mapToEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

// mapToEnvSlice converts a map of environment variables to a slice of KEY=VALUE strings
func mapToEnvSlice(env map[string]string) []string {
	if env == nil {
		return nil
	}

	envSlice := make([]string, 0, len(env))
	for k, v := range env {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}
	return envSlice
}
