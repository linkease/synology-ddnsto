package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*app, *bool, *[]string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ddnsto-config.json"), []byte(`{"token":"old-token","routeridx":7,"supplierCode":"keep-me"}`), 0600); err != nil {
		t.Fatal(err)
	}
	running := true
	calls := []string{}
	a := &app{dataDir: dir, authenticate: func(*http.Request) (string, error) { return "admin", nil }, control: func(action string) (bool, error) {
		calls = append(calls, action)
		switch action {
		case "start":
			running = true
		case "stop":
			running = false
		}
		return running, nil
	}}
	return a, &running, &calls
}

func getPage(t *testing.T, a *app) string {
	t.Helper()
	r := httptest.NewRequest("GET", "http://nas/webman/3rdparty/ddnsto/settings.cgi", nil)
	r.Header.Set("Cookie", "id=fixture-session")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("GET: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("secret response may be cached")
	}
	return w.Body.String()
}
func hidden(t *testing.T, page, name string) string {
	t.Helper()
	match := regexp.MustCompile(`name="` + name + `" value="([^"]+)"`).FindStringSubmatch(page)
	if len(match) != 2 {
		t.Fatalf("missing form field %s", name)
	}
	return match[1]
}
func post(t *testing.T, a *app, page, token, index string) *httptest.ResponseRecorder {
	t.Helper()
	data := url.Values{"csrf": {hidden(t, page, "csrf")}, "revision": {hidden(t, page, "revision")}, "token": {token}, "routeridx": {index}}
	r := httptest.NewRequest("POST", "http://nas/webman/3rdparty/ddnsto/settings.cgi", strings.NewReader(data.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Cookie", "id=fixture-session")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	return w
}
func TestSettingsReadSaveAndRestart(t *testing.T) {
	a, _, calls := fixture(t)
	page := getPage(t, a)
	if !strings.Contains(page, `type="password"`) || !strings.Contains(page, `value="old-token"`) {
		t.Fatal("cannot view current token")
	}
	w := post(t, a, page, "new-token", "12")
	if w.Code != 303 {
		t.Fatalf("POST: %d %s", w.Code, w.Body.String())
	}
	b, _ := os.ReadFile(filepath.Join(a.dataDir, "ddnsto-config.json"))
	s := string(b)
	if !strings.Contains(s, "new-token") || !strings.Contains(s, "keep-me") || !strings.Contains(s, `12`) {
		t.Fatal("configuration not preserved/saved")
	}
	if strings.Join(*calls, ",") != "status,status,stop,start" {
		t.Fatalf("control: %v", *calls)
	}
	info, _ := os.Stat(filepath.Join(a.dataDir, "ddnsto-config.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("config permissions")
	}
}
func TestSettingsStoppedStaysStopped(t *testing.T) {
	a, running, calls := fixture(t)
	*running = false
	page := getPage(t, a)
	if w := post(t, a, page, "new-token", "0"); w.Code != 303 {
		t.Fatal(w.Code)
	}
	if *running || strings.Join(*calls, ",") != "status,status" {
		t.Fatal("stopped service was started")
	}
}
func TestSettingsRejectsUnauthorizedAndCSRF(t *testing.T) {
	a, _, calls := fixture(t)
	a.authenticate = func(*http.Request) (string, error) { return "", errors.New("not administrator") }
	w := httptest.NewRecorder()
	a.ServeHTTP(w, httptest.NewRequest("GET", "http://nas/", nil))
	if w.Code != 403 || strings.Contains(w.Body.String(), "old-token") || len(*calls) != 0 {
		t.Fatal("unauthorized access")
	}
	a.authenticate = func(*http.Request) (string, error) { return "admin", nil }
	page := getPage(t, a)
	page = strings.Replace(page, hidden(t, page, "csrf"), "invalid", 1)
	if w := post(t, a, page, "new-token", "0"); w.Code != 403 {
		t.Fatal("missing CSRF validation", w.Code)
	}
	b, _ := os.ReadFile(filepath.Join(a.dataDir, "ddnsto-config.json"))
	if strings.Contains(string(b), "new-token") {
		t.Fatal("unauthorized write")
	}
}
func TestSettingsRejectsInvalidAndStaleUpdates(t *testing.T) {
	for _, test := range []struct{ token, index string }{{"", "0"}, {"bad token", "0"}, {"new-token", "-1"}, {"new-token", "100"}, {"new-token", "1.5"}} {
		a, _, _ := fixture(t)
		page := getPage(t, a)
		if w := post(t, a, page, test.token, test.index); w.Code != 400 {
			t.Fatalf("invalid update allowed: %+v code=%d", test, w.Code)
		}
	}
	a, _, _ := fixture(t)
	page := getPage(t, a)
	if w := post(t, a, page, "first-update", "0"); w.Code != 303 {
		t.Fatal(w.Code)
	}
	if w := post(t, a, page, "stale-update", "0"); w.Code != 409 {
		t.Fatal("stale form allowed", w.Code)
	}
}
func TestSettingsRollsBackFailedRestart(t *testing.T) {
	a, _, _ := fixture(t)
	original, _ := os.ReadFile(filepath.Join(a.dataDir, "ddnsto-config.json"))
	boot := filepath.Join(a.dataDir, ".ddnsto_boot.cfg")
	os.WriteFile(boot, []byte("old-boot-state"), 0600)
	page := getPage(t, a)
	starts := 0
	a.control = func(action string) (bool, error) {
		if action == "start" {
			starts++
			if starts == 1 {
				return false, errors.New("start failed")
			}
		}
		return true, nil
	}
	w := post(t, a, page, "new-token", "8")
	if w.Code != 500 {
		t.Fatal(w.Code)
	}
	got, _ := os.ReadFile(filepath.Join(a.dataDir, "ddnsto-config.json"))
	if string(got) != string(original) {
		t.Fatal("original config lost")
	}
	state, _ := os.ReadFile(boot)
	if string(state) != "old-boot-state" || starts != 2 {
		t.Fatal("old service not recovered")
	}
}

// Test-only browser harness; never included in the packaged CGI binary.
func TestBrowserPreview(t *testing.T) {
	if os.Getenv("DDNSTO_SETTINGS_PREVIEW") != "1" {
		t.Skip("manual browser integration")
	}
	a, _, _ := fixture(t)
	a.authenticate = func(*http.Request) (string, error) { return "preview-admin", nil }
	done := make(chan struct{})
	listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__test_shutdown" {
			close(done)
			return
		}
		a.ServeHTTP(w, r)
	}))
	defer listener.Close()
	os.WriteFile(os.Getenv("DDNSTO_PREVIEW_URL_FILE"), []byte(listener.URL), 0600)
	select {
	case <-done:
	case <-time.After(10 * time.Minute):
	}
}

func TestDSMAuthenticationRequiresAdminGroup(t *testing.T) {
	dir := t.TempDir()
	auth := filepath.Join(dir, "authenticate.cgi")
	id := filepath.Join(dir, "id")
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write(auth, "#!/bin/sh\n[ \"$HTTP_COOKIE\" = \"id=fixture-session\" ] || exit 1\nprintf fixture-admin\n")
	write(id, "#!/bin/sh\nprintf 'users administrators'\n")
	t.Setenv("HTTP_COOKIE", "id=fixture-session")
	req := httptest.NewRequest("GET", "http://nas/", nil)
	if user, err := authenticateCommands(req, auth, id); err != nil || user != "fixture-admin" {
		t.Fatalf("valid DSM login rejected: %s %v", user, err)
	}
	write(id, "#!/bin/sh\nprintf users\n")
	if _, err := authenticateCommands(req, auth, id); err == nil {
		t.Fatal("non-administrator allowed")
	}
	t.Setenv("HTTP_COOKIE", "id=invalid-session")
	if _, err := authenticateCommands(req, auth, id); err == nil {
		t.Fatal("invalid session allowed")
	}
}

func TestSettingsRedirectAfterSave(t *testing.T) {
	a, _, _ := fixture(t)
	page := getPage(t, a)
	w := post(t, a, page, "new-token", "0")
	if w.Code != 303 || w.Header().Get("Location") != "/webman/3rdparty/ddnsto/settings.cgi?result=saved" {
		t.Fatalf("saving must redirect to a refresh-safe GET: %d %s", w.Code, w.Header().Get("Location"))
	}
	page = getPage(t, a)
	if !strings.Contains(page, `value="new-token"`) {
		t.Fatal("saved value not visible on reload")
	}
}

func TestRuntimeStatusAndLogs(t *testing.T) {
	a, running, _ := fixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Error(r.URL.Path)
		}
		w.Write([]byte(`{"connected":true,"routerId":"device-123","version":"4.2.7"}`))
	}))
	defer server.Close()
	a.statusPath = filepath.Join(a.dataDir, "addr.json")
	os.WriteFile(a.statusPath, []byte(`{"addr":"`+strings.TrimPrefix(server.URL, "http://")+`","pid":123}`), 0600)
	os.WriteFile(filepath.Join(a.dataDir, "ddnsto.pid"), []byte("123"), 0600)
	os.Mkdir(filepath.Join(a.dataDir, "logs"), 0700)
	os.WriteFile(filepath.Join(a.dataDir, "logs/ddnstoshell.log"), []byte("connect failed token=old-token <script>alert(1)</script>"), 0600)
	page := getPage(t, a)
	for _, want := range []string{"device-123", "已连接服务端", "connect failed", "[REDACTED]", "https://web.ddnsto.com/"} {
		if !strings.Contains(page, want) {
			t.Error("missing", want)
		}
	}
	if strings.Contains(page, "token=old-token") || strings.Contains(page, "<script>alert(1)</script>") {
		t.Fatal("unsafe log rendering")
	}
	os.WriteFile(filepath.Join(a.dataDir, "ddnsto.pid"), []byte("456"), 0600)
	if strings.Contains(getPage(t, a), "device-123") {
		t.Fatal("stale status accepted")
	}
	*running = false
	if strings.Contains(getPage(t, a), "已连接服务端") {
		t.Fatal("stopped process shown online")
	}
}
func TestLogTailBounded(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log")
	os.WriteFile(p, []byte(strings.Repeat("x", 100000)+"\nlast line"), 0600)
	got := readLogTail(p, "secret")
	if len(got) > 66000 || !strings.Contains(got, "last line") {
		t.Fatal("unbounded or incomplete log tail")
	}
}
