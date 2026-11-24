package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/stretchr/testify/require"
)

// Contract: plain environment settings replace packaging config seeding.
func TestDeploymentEnvironmentOverridesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte(`listen = "127.0.0.1:7000"
[web]
listen = "127.0.0.1:7001"
public_origin = "https://old.example"
[search.embeddings]
base_url = "https://old.example/v1"
model = "old-model"
dims = 128
[mcp]
enabled = false
actor = "old-agent"
projects = ["old-project"]
startup_sections = ["activity"]
token_file = "/run/secrets/old-mcp-token"
`), 0o600))
	t.Setenv("POOMSAE_LISTEN", " 127.0.0.1:7777 ")
	t.Setenv("POOMSAE_WEB_LISTEN", "127.0.0.1:7778")
	t.Setenv("POOMSAE_WEB_PUBLIC_ORIGIN", "http://daemon.example:80")
	t.Setenv("POOMSAE_TRUST_PRIVATE_NETWORK", "1")
	t.Setenv("POOMSAE_SEARCH_EMBEDDINGS_BASE_URL", "https://embedding.example/v1")
	t.Setenv("POOMSAE_SEARCH_EMBEDDINGS_MODEL", "example-model")
	t.Setenv("POOMSAE_SEARCH_EMBEDDINGS_DIMS", "1024")
	t.Setenv("POOMSAE_SEARCH_EMBEDDINGS_API_KEY_FILE", "/run/secrets/embedding-key")
	t.Setenv("POOMSAE_DOCBANK_URL", "https://docbank.example")
	t.Setenv("POOMSAE_DOCBANK_API_KEY_FILE", "/run/secrets/docbank-key")
	t.Setenv("POOMSAE_DOCBANK_FOLDER", "/issues")
	t.Setenv("POOMSAE_MCP_ENABLED", " true ")
	t.Setenv("POOMSAE_MCP_ALL_PROJECTS", " true ")
	t.Setenv("POOMSAE_MCP_ACTOR", " gateway-agent ")
	t.Setenv("POOMSAE_MCP_PROJECTS", " alpha, beta ")
	t.Setenv("POOMSAE_MCP_STARTUP_SECTIONS", " issue_discovery, docs ")
	t.Setenv("POOMSAE_MCP_TOKEN_FILE", " /run/secrets/mcp-token ")
	cfg, err := config.ReadDaemonConfig()
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:7777", cfg.Listen)
	require.Equal(t, "127.0.0.1:7778", cfg.Web.Listen)
	require.Equal(t, "http://daemon.example", cfg.Web.PublicOrigin)
	require.Equal(t, "https://embedding.example/v1", cfg.Search.Embeddings.BaseURL)
	require.Equal(t, "example-model", cfg.Search.Embeddings.Model)
	require.Equal(t, 1024, cfg.Search.Embeddings.Dims)
	require.Equal(t, "/run/secrets/embedding-key", cfg.Search.Embeddings.APIKeyFile)
	require.Equal(t, "/issues", cfg.Docbank.Folder)
	require.Equal(t, "https://docbank.example", cfg.Docbank.URL)
	require.Equal(t, "/run/secrets/docbank-key", cfg.Docbank.APIKeyFile)
	require.True(t, cfg.MCP.Enabled)
	require.True(t, cfg.MCP.AllProjects)
	require.Equal(t, "gateway-agent", cfg.MCP.Actor)
	require.Equal(t, []string{"alpha", "beta"}, cfg.MCP.Projects)
	require.Equal(t, []string{"issue_discovery", "docs"}, cfg.MCP.StartupSections)
	require.Equal(t, "POOMSAE_MCP_STARTUP_SECTIONS", cfg.Sources["mcp_startup_sections"])
	require.Equal(t, "/run/secrets/mcp-token", cfg.MCP.TokenFile)
	for _, tt := range []struct {
		name, value string
		want        []string
	}{
		{name: "whitespace", value: " issue_discovery, issue_mutation, coordination, issue_lifecycle, projects, docs ", want: []string{"issue_discovery", "issue_mutation", "coordination", "issue_lifecycle", "projects", "docs"}},
		{name: "duplicate", value: "docs, docs ", want: []string{"docs", "docs"}},
		{name: "trailing comma", value: "docs,", want: []string{"docs", ""}},
		{name: "repeated comma", value: "docs,,activity", want: []string{"docs", "", "activity"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("POOMSAE_MCP_STARTUP_SECTIONS", tt.value)
			cfg, err := config.ReadDaemonConfig()
			require.NoError(t, err)
			require.Equal(t, tt.want, cfg.MCP.StartupSections)
			require.Equal(t, "POOMSAE_MCP_STARTUP_SECTIONS", cfg.Sources["mcp_startup_sections"])
		})
	}
	auth, err := config.ReadAuthConfig()
	require.NoError(t, err)
	require.True(t, auth.TrustPrivateNetwork)
}

func TestDeploymentInvalidEmbeddingDimensionsFailLoudly(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	for _, value := range []string{"wrong", "-1", "92233720368547758070"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("POOMSAE_SEARCH_EMBEDDINGS_DIMS", value)
			_, err := config.ReadDaemonConfig()
			require.Error(t, err)
			require.Contains(t, err.Error(), "DIMS")
		})
	}
}

// Contract: container env can carry literal role prompts, whose leading and
// trailing spaces are part of the text sent to the provider.
func TestDeploymentEmbeddingTextControlsOverrideFileVerbatim(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte(`[search.embeddings]
base_url = "https://embedding.example/v1"
model = "example-model"
document_prefix = "file document: "
document_suffix = " file end"
query_prefix = "file query: "
query_suffix = " file end"
request_dimensions = true
`), 0o600))
	t.Setenv("POOMSAE_SEARCH_EMBEDDINGS_DOCUMENT_PREFIX", "title: none | text: ")
	t.Setenv("POOMSAE_SEARCH_EMBEDDINGS_DOCUMENT_SUFFIX", " document end ")
	t.Setenv("POOMSAE_SEARCH_EMBEDDINGS_QUERY_PREFIX", "task: search result | query: ")
	t.Setenv("POOMSAE_SEARCH_EMBEDDINGS_QUERY_SUFFIX", " query end ")
	t.Setenv("POOMSAE_SEARCH_EMBEDDINGS_REQUEST_DIMENSIONS", " false ")
	cfg, err := config.ReadDaemonConfig()
	require.NoError(t, err)
	ec := cfg.Search.Embeddings
	require.Equal(t, "title: none | text: ", ec.DocumentPrefix)
	require.Equal(t, " document end ", ec.DocumentSuffix)
	require.Equal(t, "task: search result | query: ", ec.QueryPrefix)
	require.Equal(t, " query end ", ec.QuerySuffix)
	require.False(t, ec.RequestDimensions)
}

func TestDeploymentInvalidBoolEnvFailsLoudly(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	for _, name := range []string{"POOMSAE_SEARCH_EMBEDDINGS_REQUEST_DIMENSIONS", "POOMSAE_MCP_ENABLED", "POOMSAE_MCP_ALL_PROJECTS"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "sometimes")
			_, err := config.ReadDaemonConfig()
			require.ErrorContains(t, err, name)
		})
	}
}

func TestLocalProfileRejectsDeploymentControlReferences(t *testing.T) {
	for _, key := range []string{"POOMSAE_LISTEN", "POOMSAE_WEB_LISTEN", "POOMSAE_WEB_PUBLIC_ORIGIN", "POOMSAE_MCP_ENABLED", "POOMSAE_MCP_ACTOR", "POOMSAE_MCP_PROJECTS", "POOMSAE_MCP_ALL_PROJECTS", "POOMSAE_MCP_STARTUP_SECTIONS", "POOMSAE_MCP_TOKEN_FILE", "POOMSAE_AUTH_TOKEN_FILE", "POOMSAE_DOCBANK_URL", "POOMSAE_DOCBANK_API_KEY_FILE", "POOMSAE_DOCBANK_FOLDER", "POOMSAE_DOCBANK_TRUST_PRIVATE_NETWORK", "POOMSAE_SEARCH_EMBEDDINGS_BASE_URL", "POOMSAE_SEARCH_EMBEDDINGS_MODEL", "POOMSAE_SEARCH_EMBEDDINGS_DIMS", "POOMSAE_SEARCH_EMBEDDINGS_API_KEY_FILE", "POOMSAE_SEARCH_EMBEDDINGS_DOCUMENT_PREFIX", "POOMSAE_SEARCH_EMBEDDINGS_DOCUMENT_SUFFIX", "POOMSAE_SEARCH_EMBEDDINGS_QUERY_PREFIX", "POOMSAE_SEARCH_EMBEDDINGS_QUERY_SUFFIX", "POOMSAE_SEARCH_EMBEDDINGS_REQUEST_DIMENSIONS", "POOMSAE_BACKUP_DIR", "POOMSAE_BACKUP_INTERVAL", "POOMSAE_BACKUP_RETAIN"} {
		t.Run(key, func(t *testing.T) {
			_, err := config.LocalProfileEnvironment(config.LocalProfileConfig{Home: t.TempDir(), Catalog: config.CatalogDaemonConfig{TokenEnv: key}}, true)
			require.ErrorContains(t, err, "reserved environment key")
		})
	}
}

// Empty Compose substitutions must preserve the operator's TOML settings.
func TestDeploymentEmptyEnvironmentPreservesFile(t *testing.T) {
	for _, value := range []string{"", " \t "} {
		t.Run(value, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("POOMSAE_HOME", home)
			require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte(`listen = "127.0.0.1:7000"
[web]
listen = "127.0.0.1:7001"
public_origin = "https://daemon.example"
[search.embeddings]
base_url = "https://embedding.example/v1"
model = "example-model"
dims = 128
api_key_file = "/run/secrets/embedding-key"
document_prefix = "file document: "
query_prefix = "file query: "
request_dimensions = true
[backup]
dir = "/data/backups"
interval = "24h"
retain = "720h"
[mcp]
startup_sections = ["issue_discovery", "docs"]
`), 0o600))
			t.Setenv("POOMSAE_MCP_STARTUP_SECTIONS", value)
			for _, name := range []string{"POOMSAE_LISTEN", "POOMSAE_WEB_LISTEN", "POOMSAE_WEB_PUBLIC_ORIGIN", "POOMSAE_SEARCH_EMBEDDINGS_BASE_URL", "POOMSAE_SEARCH_EMBEDDINGS_MODEL", "POOMSAE_SEARCH_EMBEDDINGS_API_KEY_FILE", "POOMSAE_SEARCH_EMBEDDINGS_DIMS", "POOMSAE_SEARCH_EMBEDDINGS_DOCUMENT_PREFIX", "POOMSAE_SEARCH_EMBEDDINGS_DOCUMENT_SUFFIX", "POOMSAE_SEARCH_EMBEDDINGS_QUERY_PREFIX", "POOMSAE_SEARCH_EMBEDDINGS_QUERY_SUFFIX", "POOMSAE_SEARCH_EMBEDDINGS_REQUEST_DIMENSIONS", "POOMSAE_BACKUP_DIR", "POOMSAE_BACKUP_INTERVAL", "POOMSAE_BACKUP_RETAIN"} {
				t.Setenv(name, value)
			}
			cfg, err := config.ReadDaemonConfig()
			require.NoError(t, err)
			require.Equal(t, "127.0.0.1:7000", cfg.Listen)
			require.Equal(t, "127.0.0.1:7001", cfg.Web.Listen)
			require.Equal(t, "https://daemon.example", cfg.Web.PublicOrigin)
			require.Equal(t, "https://embedding.example/v1", cfg.Search.Embeddings.BaseURL)
			require.Equal(t, "example-model", cfg.Search.Embeddings.Model)
			require.Equal(t, 128, cfg.Search.Embeddings.Dims)
			require.Equal(t, "/run/secrets/embedding-key", cfg.Search.Embeddings.APIKeyFile)
			require.Equal(t, "file document: ", cfg.Search.Embeddings.DocumentPrefix)
			require.Equal(t, "file query: ", cfg.Search.Embeddings.QueryPrefix)
			require.True(t, cfg.Search.Embeddings.RequestDimensions)
			require.Equal(t, config.BackupConfig{Dir: "/data/backups", Interval: "24h", Retain: "720h"}, cfg.Backup)
			require.Equal(t, []string{"issue_discovery", "docs"}, cfg.MCP.StartupSections)
		})
	}
}

func TestDeploymentDocbankPrivateNetworkEnvironment(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DOCBANK_URL", "http://docbank:8485")
	t.Setenv("POOMSAE_DOCBANK_API_KEY_FILE", "/run/secrets/docbank-key")
	t.Setenv("POOMSAE_DOCBANK_TRUST_PRIVATE_NETWORK", "true")
	cfg, err := config.ReadDaemonConfig()
	require.NoError(t, err)
	require.True(t, cfg.Docbank.TrustPrivateNetwork)
}
