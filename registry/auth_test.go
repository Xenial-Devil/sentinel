package registry

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// makeDockerConfig writes a config.json to a temp dir and returns the dir path.
// entries maps registry → "username:password"
func makeDockerConfig(t *testing.T, entries map[string]string) string {
	t.Helper()
	auths := make(map[string]AuthEntry)
	for reg, raw := range entries {
		auths[reg] = AuthEntry{Auth: base64.StdEncoding.EncodeToString([]byte(raw))}
	}
	data, err := json.Marshal(DockerConfig{Auths: auths})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return dir
}

func clearGenericCreds(t *testing.T) {
	t.Helper()
	os.Unsetenv("REPO_USER")
	os.Unsetenv("REPO_PASS")
}

func clearPerRegistryCreds(t *testing.T, reg string) {
	t.Helper()
	normalized := strings.ToUpper(reg)
	normalized = strings.NewReplacer(".", "_", ":", "_", "-", "_").Replace(normalized)
	os.Unsetenv("SENTINEL_REGISTRY_USER_" + normalized)
	os.Unsetenv("SENTINEL_REGISTRY_PASS_" + normalized)
	os.Unsetenv("SENTINEL_REGISTRY_TOKEN_" + normalized)
}

// ── splitCredentials ──────────────────────────────────────────────────────────

func TestSplitCredentials_Normal(t *testing.T) {
	got := splitCredentials("alice:secret")
	if len(got) != 2 || got[0] != "alice" || got[1] != "secret" {
		t.Errorf("got %v", got)
	}
}

func TestSplitCredentials_PasswordHasColon(t *testing.T) {
	// Must split on FIRST colon only
	got := splitCredentials("user:pass:word")
	if len(got) != 2 || got[0] != "user" || got[1] != "pass:word" {
		t.Errorf("got %v, want [user pass:word]", got)
	}
}

func TestSplitCredentials_NoColon(t *testing.T) {
	if got := splitCredentials("nocoion"); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestSplitCredentials_Empty(t *testing.T) {
	if got := splitCredentials(""); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestSplitCredentials_OnlyColon(t *testing.T) {
	got := splitCredentials(":")
	if len(got) != 2 || got[0] != "" || got[1] != "" {
		t.Errorf("got %v, want two empty strings", got)
	}
}

// ── getGenericEnvCredentials ─────────────────────────────────────────────────

func TestGenericEnv_BothSet(t *testing.T) {
	t.Setenv("REPO_USER", "u")
	t.Setenv("REPO_PASS", "p")
	c := getGenericEnvCredentials()
	if c == nil || c.Username != "u" || c.Password != "p" {
		t.Errorf("got %v", c)
	}
}

func TestGenericEnv_OnlyUser(t *testing.T) {
	t.Setenv("REPO_USER", "u")
	os.Unsetenv("REPO_PASS")
	if c := getGenericEnvCredentials(); c != nil {
		t.Errorf("expected nil, got %v", c)
	}
}

func TestGenericEnv_OnlyPass(t *testing.T) {
	os.Unsetenv("REPO_USER")
	t.Setenv("REPO_PASS", "p")
	if c := getGenericEnvCredentials(); c != nil {
		t.Errorf("expected nil, got %v", c)
	}
}

func TestGenericEnv_NoneSet(t *testing.T) {
	os.Unsetenv("REPO_USER")
	os.Unsetenv("REPO_PASS")
	if c := getGenericEnvCredentials(); c != nil {
		t.Errorf("expected nil, got %v", c)
	}
}

func TestGenericEnv_EmptyStrings(t *testing.T) {
	t.Setenv("REPO_USER", "")
	t.Setenv("REPO_PASS", "")
	if c := getGenericEnvCredentials(); c != nil {
		t.Errorf("expected nil for empty strings, got %v", c)
	}
}

// ── getPerRegistryEnvCredentials ─────────────────────────────────────────────

func TestPerRegistry_UserPass(t *testing.T) {
	t.Setenv("SENTINEL_REGISTRY_USER_GHCR_IO", "gu")
	t.Setenv("SENTINEL_REGISTRY_PASS_GHCR_IO", "gp")
	c := getPerRegistryEnvCredentials("ghcr.io")
	if c == nil || c.Username != "gu" || c.Password != "gp" {
		t.Errorf("got %v", c)
	}
}

func TestPerRegistry_TokenVariant(t *testing.T) {
	clearPerRegistryCreds(t, "ghcr.io")
	t.Setenv("SENTINEL_REGISTRY_TOKEN_GHCR_IO", "tok123")
	c := getPerRegistryEnvCredentials("ghcr.io")
	if c == nil || c.Username != "token" || c.Password != "tok123" {
		t.Errorf("got %v", c)
	}
}

func TestPerRegistry_OnlyUserNoPass(t *testing.T) {
	t.Setenv("SENTINEL_REGISTRY_USER_GHCR_IO", "u")
	os.Unsetenv("SENTINEL_REGISTRY_PASS_GHCR_IO")
	os.Unsetenv("SENTINEL_REGISTRY_TOKEN_GHCR_IO")
	if c := getPerRegistryEnvCredentials("ghcr.io"); c != nil {
		t.Errorf("expected nil, got %v", c)
	}
}

func TestPerRegistry_NormalizationDots(t *testing.T) {
	// registry.example.com → REGISTRY_EXAMPLE_COM
	t.Setenv("SENTINEL_REGISTRY_USER_REGISTRY_EXAMPLE_COM", "u")
	t.Setenv("SENTINEL_REGISTRY_PASS_REGISTRY_EXAMPLE_COM", "p")
	c := getPerRegistryEnvCredentials("registry.example.com")
	if c == nil || c.Username != "u" {
		t.Errorf("got %v", c)
	}
}

func TestPerRegistry_NormalizationColonPort(t *testing.T) {
	// my-reg:5000 → MY_REG_5000
	t.Setenv("SENTINEL_REGISTRY_USER_MY_REG_5000", "u")
	t.Setenv("SENTINEL_REGISTRY_PASS_MY_REG_5000", "p")
	c := getPerRegistryEnvCredentials("my-reg:5000")
	if c == nil || c.Username != "u" {
		t.Errorf("got %v", c)
	}
}

func TestPerRegistry_Unknown(t *testing.T) {
	clearPerRegistryCreds(t, "unknown.reg.io")
	if c := getPerRegistryEnvCredentials("unknown.reg.io"); c != nil {
		t.Errorf("expected nil, got %v", c)
	}
}

// ── getDockerConfigPath ───────────────────────────────────────────────────────

func TestDockerConfigPath_EnvVarOverride(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", "/custom/dir")
	got := filepath.ToSlash(getDockerConfigPath())
	if got != "/custom/dir/config.json" {
		t.Errorf("got %q", got)
	}
}

func TestDockerConfigPath_EmptyEnvFallsToOS(t *testing.T) {
	os.Unsetenv("DOCKER_CONFIG")
	got := getDockerConfigPath()
	if !strings.HasSuffix(got, "config.json") {
		t.Errorf("want ...config.json, got %q", got)
	}
	if !strings.Contains(got, ".docker") {
		t.Errorf("want .docker in path, got %q", got)
	}
}

func TestDockerConfigPath_WindowsDefault(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only")
	}
	os.Unsetenv("DOCKER_CONFIG")
	got := getDockerConfigPath()
	if !strings.Contains(got, os.Getenv("USERPROFILE")) {
		t.Errorf("expected USERPROFILE in path, got %q", got)
	}
}

func TestDockerConfigPath_UnixDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only")
	}
	os.Unsetenv("DOCKER_CONFIG")
	got := getDockerConfigPath()
	if !strings.Contains(got, os.Getenv("HOME")) {
		t.Errorf("expected HOME in path, got %q", got)
	}
}

// ── getDockerConfigCredentials ────────────────────────────────────────────────

func TestDockerConfig_ValidEntry(t *testing.T) {
	dir := makeDockerConfig(t, map[string]string{"ghcr.io": "u:p"})
	t.Setenv("DOCKER_CONFIG", dir)
	c, err := getDockerConfigCredentials("ghcr.io")
	if err != nil || c == nil || c.Username != "u" || c.Password != "p" {
		t.Errorf("err=%v creds=%v", err, c)
	}
}

func TestDockerConfig_RegistryAbsent(t *testing.T) {
	dir := makeDockerConfig(t, map[string]string{"docker.io": "u:p"})
	t.Setenv("DOCKER_CONFIG", dir)
	c, err := getDockerConfigCredentials("ghcr.io")
	if err != nil || c != nil {
		t.Errorf("expected nil creds for missing registry; err=%v creds=%v", err, c)
	}
}

func TestDockerConfig_MissingFile(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir()) // valid dir, no config.json
	c, err := getDockerConfigCredentials("ghcr.io")
	if err != nil || c != nil {
		t.Errorf("expected nil on missing file; err=%v creds=%v", err, c)
	}
}

func TestDockerConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("not-json"), 0600)
	t.Setenv("DOCKER_CONFIG", dir)
	_, err := getDockerConfigCredentials("ghcr.io")
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected parse error, got %v", err)
	}
}

func TestDockerConfig_InvalidBase64(t *testing.T) {
	dir := t.TempDir()
	cfg := DockerConfig{Auths: map[string]AuthEntry{"ghcr.io": {Auth: "!!!notbase64"}}}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dir, "config.json"), data, 0600)
	t.Setenv("DOCKER_CONFIG", dir)
	_, err := getDockerConfigCredentials("ghcr.io")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestDockerConfig_NoColonInDecoded(t *testing.T) {
	dir := t.TempDir()
	// base64 of "nocolon" (no colon → splitCredentials returns nil)
	encoded := base64.StdEncoding.EncodeToString([]byte("nocolon"))
	cfg := DockerConfig{Auths: map[string]AuthEntry{"ghcr.io": {Auth: encoded}}}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dir, "config.json"), data, 0600)
	t.Setenv("DOCKER_CONFIG", dir)
	_, err := getDockerConfigCredentials("ghcr.io")
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected invalid format error, got %v", err)
	}
}

// ── GetCredentials – priority ─────────────────────────────────────────────────

func TestGetCredentials_RepoEnvBeatsAll(t *testing.T) {
	dir := makeDockerConfig(t, map[string]string{"ghcr.io": "dockeru:dockerp"})
	t.Setenv("DOCKER_CONFIG", dir)
	t.Setenv("REPO_USER", "repoU")
	t.Setenv("REPO_PASS", "repoP")
	t.Setenv("SENTINEL_REGISTRY_USER_GHCR_IO", "perU")
	t.Setenv("SENTINEL_REGISTRY_PASS_GHCR_IO", "perP")

	c, err := GetCredentials("ghcr.io")
	if err != nil || c == nil || c.Username != "repoU" {
		t.Errorf("REPO_USER should win; got %v err=%v", c, err)
	}
}

func TestGetCredentials_PerRegistryBeatsDocker(t *testing.T) {
	dir := makeDockerConfig(t, map[string]string{"ghcr.io": "dockeru:dockerp"})
	t.Setenv("DOCKER_CONFIG", dir)
	clearGenericCreds(t)
	t.Setenv("SENTINEL_REGISTRY_USER_GHCR_IO", "perU")
	t.Setenv("SENTINEL_REGISTRY_PASS_GHCR_IO", "perP")

	c, err := GetCredentials("ghcr.io")
	if err != nil || c == nil || c.Username != "perU" {
		t.Errorf("per-registry should win; got %v err=%v", c, err)
	}
}

func TestGetCredentials_FallsBackToDockerConfig(t *testing.T) {
	dir := makeDockerConfig(t, map[string]string{"ghcr.io": "dockeru:dockerp"})
	t.Setenv("DOCKER_CONFIG", dir)
	clearGenericCreds(t)
	clearPerRegistryCreds(t, "ghcr.io")

	c, err := GetCredentials("ghcr.io")
	if err != nil || c == nil || c.Username != "dockeru" {
		t.Errorf("should fall back to docker config; got %v err=%v", c, err)
	}
}

func TestGetCredentials_NoneConfigured(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	clearGenericCreds(t)
	clearPerRegistryCreds(t, "ghcr.io")

	c, err := GetCredentials("ghcr.io")
	if err != nil || c != nil {
		t.Errorf("expected nil; got %v err=%v", c, err)
	}
}

// ── GetAuthHeader ─────────────────────────────────────────────────────────────

func TestGetAuthHeader_WithCreds(t *testing.T) {
	t.Setenv("REPO_USER", "u")
	t.Setenv("REPO_PASS", "p")
	h := GetAuthHeader("ghcr.io")
	if h == "" {
		t.Fatal("expected non-empty")
	}
	raw, err := base64.URLEncoding.DecodeString(h)
	if err != nil {
		t.Fatalf("not valid base64url: %v", err)
	}
	if !strings.Contains(string(raw), `"username":"u"`) || !strings.Contains(string(raw), `"password":"p"`) {
		t.Errorf("bad json in header: %s", raw)
	}
}

func TestGetAuthHeader_NoCreds(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	clearGenericCreds(t)
	clearPerRegistryCreds(t, "ghcr.io")
	if h := GetAuthHeader("ghcr.io"); h != "" {
		t.Errorf("expected empty, got %q", h)
	}
}

// ── GetBasicAuthHeader ────────────────────────────────────────────────────────

func TestGetBasicAuthHeader_WithCreds(t *testing.T) {
	t.Setenv("REPO_USER", "alice")
	t.Setenv("REPO_PASS", "secret")
	h := GetBasicAuthHeader("ghcr.io")
	if !strings.HasPrefix(h, "Basic ") {
		t.Fatalf("expected Basic prefix, got %q", h)
	}
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(h, "Basic "))
	if string(raw) != "alice:secret" {
		t.Errorf("decoded = %q", raw)
	}
}

func TestGetBasicAuthHeader_NoCreds(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	clearGenericCreds(t)
	clearPerRegistryCreds(t, "ghcr.io")
	if h := GetBasicAuthHeader("ghcr.io"); h != "" {
		t.Errorf("expected empty, got %q", h)
	}
}
