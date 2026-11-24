package config

import (
	"fmt"
	"os"
	"strings"
)

// legacyEnvSkip are kata variables the migration handles itself. Mapping
// KATA_HOME or KATA_DB onto the poomsae names would keep writing the original
// files instead of the copy.
var legacyEnvSkip = map[string]struct{}{
	"KATA_HOME": {},
	"KATA_DB":   {},
	"KATA_DSN":  {},
}

// ApplyLegacyKataEnv copies KATA_* variables to the matching POOMSAE_* names
// when the poomsae variable is unset. KATA_HOME, KATA_DB, and KATA_DSN are
// left for the data migration. It is safe to call more than once.
func ApplyLegacyKataEnv() []string {
	var mapped []string
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(key, "KATA_") {
			continue
		}
		if _, skip := legacyEnvSkip[key]; skip {
			continue
		}
		dest := "POOMSAE_" + strings.TrimPrefix(key, "KATA_")
		if _, exists := os.LookupEnv(dest); exists {
			continue
		}
		if err := os.Setenv(dest, value); err != nil {
			continue
		}
		mapped = append(mapped, key+" -> "+dest)
	}
	return mapped
}

// LegacyEnvWarning is the stderr line for a non-empty mapping, or "".
func LegacyEnvWarning(mapped []string) string {
	if len(mapped) == 0 {
		return ""
	}
	return fmt.Sprintf("poomsae: warning: KATA_* environment variables are deprecated and were mapped to POOMSAE_*: %s", strings.Join(mapped, ", "))
}
