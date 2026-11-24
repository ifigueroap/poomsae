package telemetry

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	for _, key := range []string{"CI", "DO_NOT_TRACK"} {
		if err := os.Unsetenv(key); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

func TestSessionEndedDurationBuckets(t *testing.T) {
	for _, tc := range []struct {
		elapsed time.Duration
		bucket  string
	}{
		{time.Minute - time.Nanosecond, "under_1m"}, {time.Minute, "1_to_5m"},
		{5 * time.Minute, "5_to_30m"},
		{30 * time.Minute, "5_to_30m"}, {30*time.Minute + time.Nanosecond, "over_30m"},
	} {
		assert.Equal(t, tc.bucket, DurationBucket(tc.elapsed))
	}
}

// Exercise the standalone product wrapper. It stays disabled with or without
// a linked project key; poomsae does not emit telemetry.
func TestStandaloneTelemetryAdmission(t *testing.T) {
	scratch := t.TempDir()
	build := func(name string, args ...string) string {
		binary := filepath.Join(scratch, name)
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
		command := exec.CommandContext(t.Context(), "go", append(append([]string{"build"}, args...), "-o", binary, "./testprogram")...) //nolint:gosec // fixed tool and test-owned output
		output, err := command.CombinedOutput()
		require.NoError(t, err, string(output))
		return binary
	}
	release := build("telemetry-release")
	source := build("telemetry-source")
	for _, tc := range []struct {
		name, binary, shared, product, version, want string
		consent                                      *bool
	}{
		{"fresh home", release, "", "", "v1.2.3", "disabled", nil},
		{"consent yes", release, "", "", "v1.2.3", "disabled", new(true)},
		{"product veto", release, "", "0", "v1.2.3", "disabled", new(true)},
		{"development", release, "1", "", "dev", "disabled", nil},
		{"no project key", source, "1", "", "v1.2.3", "disabled", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			command := exec.CommandContext(t.Context(), tc.binary, "-version", tc.version) //nolint:gosec // test-owned standalone fixture
			command.Env = slices.DeleteFunc(os.Environ(), func(entry string) bool {
				key, _, _ := strings.Cut(entry, "=")
				return strings.EqualFold(key, "TELEMETRY_ENABLED") || strings.EqualFold(key, EnabledEnv)
			})
			command.Env = append(command.Env, "POOMSAE_HOME="+home, "POOMSAE_DB="+filepath.Join(home, "poomsae.db"))
			if tc.shared != "" {
				command.Env = append(command.Env, "TELEMETRY_ENABLED="+tc.shared)
			}
			if tc.product != "" {
				command.Env = append(command.Env, EnabledEnv+"="+tc.product)
			}
			output, err := command.CombinedOutput()
			require.NoError(t, err, string(output))
			require.Equal(t, tc.want, strings.TrimSpace(string(output)))
		})
	}
}

func TestReporterIsDisabled(t *testing.T) {
	reporter, err := NewReporter(Options{DistinctID: "anonymous-instance-id", Version: "v1.2.3"})
	require.NoError(t, err)
	assert.False(t, reporter.Enabled(), "no PostHog project key is linked")
	assert.True(t, reporter.EventAllowed("app_opened"))
	assert.True(t, reporter.EventAllowed("sync_failed"))
	assert.False(t, reporter.EventAllowed("not_an_event"))
	assert.Equal(t, "6-20", ProjectCountBucket(12))
	assert.Equal(t, "cli", AppOpenedSurface(map[string]any{"surface": "cli"}))
	assert.Empty(t, AppOpenedSurface(map[string]any{"surface": "desktop"}))
}
