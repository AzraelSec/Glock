package orchestrator

import (
	"fmt"
	"os"
	"slices"

	"github.com/AzraelSec/godotenv"
)

// loadEnv reads .env files from execDir and returns environment entries
// as []string in KEY=VAL format. Silently ignores errors (matches V1
// behaviour).
//
// Precedence: files are processed in the order given. When the same key
// appears in multiple files, the value from the LAST file wins. The
// returned slice contains exactly one entry per key — deduplication is
// performed automatically by the underlying map. Host values override file
// values, including explicitly empty host values.
func loadEnv(execDir string, filenames []string) []string {
	if len(filenames) == 0 {
		return nil
	}

	envMap, _ := godotenv.ReadFrom(execDir, false, filenames...)
	envSlice := make([]string, 0, len(envMap))
	for k, v := range envMap {
		if host, ok := os.LookupEnv(k); ok {
			v = host
		}
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}
	slices.Sort(envSlice)
	return envSlice
}
