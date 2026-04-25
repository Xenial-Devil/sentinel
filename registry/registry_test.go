package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const testDigest = "sha256:deadbeef"

func newClient(srv *httptest.Server) *Client {
	c := New()
	c.HTTPClient = srv.Client()
	return c
}

// ── isRegistry ────────────────────────────────────────────────────────────────

func TestIsRegistry(t *testing.T) {
	cases := []struct{ in string; want bool }{
		{"ghcr.io", true},
		{"registry:5000", true},
		{"localhost", true},
		{"nginx", false},
		{"myimage", false},
	}
	for _, tc := range cases {
		if got := isRegistry(tc.in); got != tc.want {
			t.Errorf("isRegistry(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// ── min ───────────────────────────────────────────────────────────────────────

func TestMin(t *testing.T) {
	if min(3, 5) != 3 { t.Error("3<5") }
	if min(7, 2) != 2 { t.Error("7>2") }
	if min(4, 4) != 4 { t.Error("4==4") }
}

// ── ParseImageRef ─────────────────────────────────────────────────────────────

func TestParseImageRef(t *testing.T) {
	cases := []struct{ in, reg, name, tag, digest string }{
		{"nginx", "registry-1.docker.io", "library/nginx", "latest", ""},
		{"nginx:1.25", "registry-1.docker.io", "library/nginx", "1.25", ""},
		{"user/app:v2", "registry-1.docker.io", "user/app", "v2", ""},
		{"ghcr.io/org/app:latest", "ghcr.io", "org/app", "latest", ""},
		{"ghcr.io/isubroto/city_pos_fe:latest", "ghcr.io", "isubroto/city_pos_fe", "latest", ""},
		{"ghcr.io/isubroto/city_pos_be:latest", "ghcr.io", "isubroto/city_pos_be", "latest", ""},
		{"localhost:5000/app:dev", "localhost:5000", "app", "dev", ""},
		{"registry.k8s.io/pause:3.9", "registry.k8s.io", "pause", "3.9", ""},
		{"nginx@sha256:abc", "registry-1.docker.io", "library/nginx", "latest", "sha256:abc"},
		{"ghcr.io/org/app", "ghcr.io", "org/app", "latest", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			r := ParseImageRef(tc.in)
			if r.Registry != tc.reg { t.Errorf("Registry=%q want %q", r.Registry, tc.reg) }
			if r.Name != tc.name    { t.Errorf("Name=%q want %q", r.Name, tc.name) }
			if r.Tag != tc.tag      { t.Errorf("Tag=%q want %q", r.Tag, tc.tag) }
			if r.Digest != tc.digest { t.Errorf("Digest=%q want %q", r.Digest, tc.digest) }
		})
	}
}

// ── fetchDigest ───────────────────────────────────────────────────────────────

func TestFetchDigest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer srv.Close()
	d, err := newClient(srv).fetchDigest(srv.URL, "")
	if err != nil || d != testDigest { t.Errorf("got %q %v", d, err) }
}

func TestFetchDigest_ForwardsAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer srv.Close()
	_, _ = newClient(srv).fetchDigest(srv.URL, "Bearer tok")
	if gotAuth != "Bearer tok" { t.Errorf("auth=%q", gotAuth) }
}

func TestFetchDigest_401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := newClient(srv).fetchDigest(srv.URL, "")
	if err == nil || !strings.Contains(err.Error(), "401") { t.Errorf("want 401 err, got %v", err) }
}

func TestFetchDigest_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := newClient(srv).fetchDigest(srv.URL, "")
	if err == nil || !strings.Contains(err.Error(), "404") { t.Errorf("want 404 err, got %v", err) }
}

func TestFetchDigest_NoDigestHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	_, err := newClient(srv).fetchDigest(srv.URL, "")
	if err == nil || !strings.Contains(err.Error(), "no digest") { t.Errorf("got %v", err) }
}

func TestFetchDigest_InvalidURL(t *testing.T) {
	c := New()
	_, err := c.fetchDigest("://bad-url", "")
	if err == nil { t.Error("expected error for invalid URL") }
}

// ── parseBearerChallenge ──────────────────────────────────────────────────────

func TestParseBearerChallenge_Full(t *testing.T) {
	h := `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:org/app:pull"`
	ref := ImageRef{Registry: "ghcr.io", Name: "org/app"}
	realm, svc, scope := parseBearerChallenge(h, ref)
	if realm != "https://ghcr.io/token" { t.Errorf("realm=%q", realm) }
	if svc != "ghcr.io"                 { t.Errorf("service=%q", svc) }
	if scope != "repository:org/app:pull" { t.Errorf("scope=%q", scope) }
}

func TestParseBearerChallenge_DefaultScope(t *testing.T) {
	h := `Bearer realm="https://ghcr.io/token",service="ghcr.io"`
	ref := ImageRef{Registry: "ghcr.io", Name: "org/myapp"}
	_, _, scope := parseBearerChallenge(h, ref)
	if scope != "repository:org/myapp:pull" { t.Errorf("scope=%q", scope) }
}

func TestParseBearerChallenge_NoRealm(t *testing.T) {
	h := `Bearer service="ghcr.io"`
	realm, _, _ := parseBearerChallenge(h, ImageRef{})
	if realm != "" { t.Errorf("expected empty realm, got %q", realm) }
}

func TestParseBearerChallenge_Empty(t *testing.T) {
	realm, _, _ := parseBearerChallenge("", ImageRef{Registry: "r", Name: "n"})
	if realm != "" { t.Errorf("expected empty realm") }
}

// ── getDockerHubToken ─────────────────────────────────────────────────────────

func TestGetDockerHubToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "mytoken"})
	}))
	defer srv.Close()
	old := dockerHubTokenURL
	dockerHubTokenURL = srv.URL
	defer func() { dockerHubTokenURL = old }()
	tok, err := getDockerHubToken("org/app", srv.Client())
	if err != nil || tok != "mytoken" { t.Errorf("got %q %v", tok, err) }
}

func TestGetDockerHubToken_EmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"token": ""})
	}))
	defer srv.Close()
	old := dockerHubTokenURL
	dockerHubTokenURL = srv.URL
	defer func() { dockerHubTokenURL = old }()
	_, err := getDockerHubToken("org/app", srv.Client())
	if err == nil || !strings.Contains(err.Error(), "empty token") { t.Errorf("got %v", err) }
}

func TestGetDockerHubToken_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()
	old := dockerHubTokenURL
	dockerHubTokenURL = srv.URL
	defer func() { dockerHubTokenURL = old }()
	_, err := getDockerHubToken("org/app", srv.Client())
	if err == nil { t.Error("expected JSON decode error") }
}

func TestGetDockerHubToken_RequestFails(t *testing.T) {
	old := dockerHubTokenURL
	dockerHubTokenURL = "http://127.0.0.1:0"
	defer func() { dockerHubTokenURL = old }()
	_, err := getDockerHubToken("org/app", &http.Client{})
	if err == nil { t.Error("expected connection error") }
}

// ── getDockerHubDigest ────────────────────────────────────────────────────────

func TestGetDockerHubDigest_Success(t *testing.T) {
	// One server handles both token + manifest requests
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") || r.URL.RawQuery != "" {
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
			return
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer srv.Close()

	oldT := dockerHubTokenURL
	oldM := dockerHubManifestBase
	dockerHubTokenURL = srv.URL
	dockerHubManifestBase = srv.URL + "/v2"
	defer func() { dockerHubTokenURL = oldT; dockerHubManifestBase = oldM }()

	ref := ImageRef{Registry: "registry-1.docker.io", Name: "library/nginx", Tag: "latest"}
	d, err := newClient(srv).getDockerHubDigest(ref)
	if err != nil || d != testDigest { t.Errorf("got %q %v", d, err) }
}

func TestGetDockerHubDigest_TokenFail(t *testing.T) {
	old := dockerHubTokenURL
	dockerHubTokenURL = "http://127.0.0.1:0"
	defer func() { dockerHubTokenURL = old }()
	ref := ImageRef{Name: "library/nginx", Tag: "latest"}
	_, err := New().getDockerHubDigest(ref)
	if err == nil { t.Error("expected error") }
}

// ── GetRemoteDigest – routing ─────────────────────────────────────────────────

func TestGetRemoteDigest_RoutesToDockerHub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
			return
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer srv.Close()

	oldT := dockerHubTokenURL
	oldM := dockerHubManifestBase
	dockerHubTokenURL = srv.URL
	dockerHubManifestBase = srv.URL
	defer func() { dockerHubTokenURL = oldT; dockerHubManifestBase = oldM }()

	d, err := newClient(srv).GetRemoteDigest("nginx:latest")
	if err != nil || d != testDigest { t.Errorf("got %q %v", d, err) }
}

func TestGetRemoteDigest_RoutesToPrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	// 127.x → http scheme used by getPrivateDigest
	d, err := newClient(srv).GetRemoteDigest(fmt.Sprintf("%s/org/app:latest", host))
	if err != nil || d != testDigest { t.Errorf("got %q %v", d, err) }
}

// ── getPrivateDigest – all auth paths ─────────────────────────────────────────

func TestPrivateDigest_AnonSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	ref := ImageRef{Registry: host, Name: "app", Tag: "latest"}
	d, err := newClient(srv).getPrivateDigest(ref)
	if err != nil || d != testDigest { t.Errorf("got %q %v", d, err) }
}

func TestPrivateDigest_AnonFail_NoCreds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_ = os.Unsetenv("REPO_USER")
	_ = os.Unsetenv("REPO_PASS")
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	host := strings.TrimPrefix(srv.URL, "http://")
	ref := ImageRef{Registry: host, Name: "app", Tag: "latest"}
	_, err := newClient(srv).getPrivateDigest(ref)
	if err == nil || !strings.Contains(err.Error(), "auth not configured") {
		t.Errorf("got %v", err)
	}
}

func TestPrivateDigest_BasicAuthSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer srv.Close()

	t.Setenv("REPO_USER", "u")
	t.Setenv("REPO_PASS", "p")

	host := strings.TrimPrefix(srv.URL, "http://")
	ref := ImageRef{Registry: host, Name: "app", Tag: "latest"}
	d, err := newClient(srv).getPrivateDigest(ref)
	if err != nil || d != testDigest { t.Errorf("got %q %v", d, err) }
}

func TestPrivateDigest_BearerSuccess(t *testing.T) {
	// Server: anon→401+WWW-Authenticate, basic→401, bearer→digest
	var probeCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		// Token endpoint
		if strings.HasSuffix(r.URL.Path, "/v2/") {
			probeCount++
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer realm="%s/token",service="reg"`, "http://"+r.Host))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "bearertok"})
			return
		}
		// Manifest: reject anon and basic, accept bearer
		if strings.HasPrefix(auth, "Bearer ") {
			w.Header().Set("Docker-Content-Digest", testDigest)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	t.Setenv("REPO_USER", "u")
	t.Setenv("REPO_PASS", "p")

	host := strings.TrimPrefix(srv.URL, "http://")
	ref := ImageRef{Registry: host, Name: "app", Tag: "latest"}
	d, err := newClient(srv).getPrivateDigest(ref)
	if err != nil || d != testDigest { t.Errorf("got %q %v", d, err) }
}

func TestPrivateDigest_BearerFetchFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/v2/") {
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer realm="%s/token",service="reg"`, "http://"+r.Host))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "bearertok"})
			return
		}
		// Always reject even bearer
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	t.Setenv("REPO_USER", "u")
	t.Setenv("REPO_PASS", "p")

	host := strings.TrimPrefix(srv.URL, "http://")
	ref := ImageRef{Registry: host, Name: "app", Tag: "latest"}
	_, err := newClient(srv).getPrivateDigest(ref)
	if err == nil { t.Error("expected error when bearer fetch fails") }
}

// ── getBearerToken – all paths ────────────────────────────────────────────────

func TestGetBearerToken_ProbeNon401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // not 401 → error
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	ref := ImageRef{Registry: host, Name: "app"}
	_, err := newClient(srv).getBearerToken(ref, "http", &Credentials{"u", "p"})
	if err == nil || !strings.Contains(err.Error(), "expected 401") { t.Errorf("got %v", err) }
}

func TestGetBearerToken_NoRealm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer service="reg"`) // no realm
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	ref := ImageRef{Registry: host, Name: "app"}
	_, err := newClient(srv).getBearerToken(ref, "http", &Credentials{"u", "p"})
	if err == nil || !strings.Contains(err.Error(), "no Bearer realm") { t.Errorf("got %v", err) }
}

func TestGetBearerToken_TokenEndpointNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/v2/") {
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer realm="%s/token",service="reg"`, "http://"+r.Host))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	ref := ImageRef{Registry: host, Name: "app"}
	_, err := newClient(srv).getBearerToken(ref, "http", &Credentials{"u", "p"})
	if err == nil || !strings.Contains(err.Error(), "token endpoint") { t.Errorf("got %v", err) }
}

func TestGetBearerToken_EmptyTokenResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/v2/") {
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer realm="%s/token",service="reg"`, "http://"+r.Host))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "", "access_token": ""})
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	ref := ImageRef{Registry: host, Name: "app"}
	_, err := newClient(srv).getBearerToken(ref, "http", &Credentials{"u", "p"})
	if err == nil || !strings.Contains(err.Error(), "empty token") { t.Errorf("got %v", err) }
}

func TestGetBearerToken_AccessTokenFallback(t *testing.T) {
	// token="" but access_token="xyz" → should return "xyz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/v2/") {
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer realm="%s/token",service="reg"`, "http://"+r.Host))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"token": "", "access_token": "xyz"})
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	ref := ImageRef{Registry: host, Name: "app"}
	tok, err := newClient(srv).getBearerToken(ref, "http", &Credentials{"u", "p"})
	if err != nil || tok != "xyz" { t.Errorf("got %q %v", tok, err) }
}

func TestGetBearerToken_ProbeRequestFails(t *testing.T) {
	// Use a stopped server so connection is refused
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close() // close immediately
	host := strings.TrimPrefix(srv.URL, "http://")
	ref := ImageRef{Registry: host, Name: "app"}
	_, err := New().getBearerToken(ref, "http", &Credentials{"u", "p"})
	if err == nil { t.Error("expected connection error") }
}

// ── HasUpdate ─────────────────────────────────────────────────────────────────

func TestHasUpdate_UpdateAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
			return
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer srv.Close()

	oldT := dockerHubTokenURL
	oldM := dockerHubManifestBase
	dockerHubTokenURL = srv.URL
	dockerHubManifestBase = srv.URL
	defer func() { dockerHubTokenURL = oldT; dockerHubManifestBase = oldM }()

	ok, remote, err := newClient(srv).HasUpdate("sha256:old", "nginx:latest")
	if err != nil { t.Fatalf("err=%v", err) }
	if !ok        { t.Error("expected hasUpdate=true") }
	if remote != testDigest { t.Errorf("remote=%q", remote) }
}

func TestHasUpdate_UpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
			return
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer srv.Close()

	oldT := dockerHubTokenURL
	oldM := dockerHubManifestBase
	dockerHubTokenURL = srv.URL
	dockerHubManifestBase = srv.URL
	defer func() { dockerHubTokenURL = oldT; dockerHubManifestBase = oldM }()

	ok, remote, err := newClient(srv).HasUpdate(testDigest, "nginx:latest")
	if err != nil { t.Fatalf("err=%v", err) }
	if ok         { t.Error("expected hasUpdate=false") }
	if remote != testDigest { t.Errorf("remote=%q", remote) }
}

func TestHasUpdate_RegistryError(t *testing.T) {
	old := dockerHubTokenURL
	dockerHubTokenURL = "http://127.0.0.1:0"
	defer func() { dockerHubTokenURL = old }()
	ok, _, err := New().HasUpdate("sha256:old", "nginx:latest")
	if err == nil || ok { t.Error("expected error") }
}
