// DDNSTO settings runs as a DSM CGI, with only the package user's privileges.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/http/cgi"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed page.html
var pageHTML string
var pageTemplate = template.Must(template.New("settings").Parse(pageHTML))
var validToken = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

const packageRoot = "/var/packages/ddnsto"
const authCGI = "/usr/syno/synoman/webman/modules/authenticate.cgi"

type app struct {
	dataDir      string
	statusPath   string
	authenticate func(*http.Request) (string, error)
	control      func(string) (bool, error)
}
type pageData struct {
	User, Token, CSRF, Revision, Nonce, Message  string
	Connection, DeviceID, Version, Logs, LogPath string
	Index                                        int
	Running, Failed                              bool
}

func digest(data []byte) string { v := sha256.Sum256(data); return hex.EncodeToString(v[:]) }
func signature(key []byte, user, cookie string) string {
	h := hmac.New(sha256.New, key)
	fmt.Fprintf(h, "%s\x00%s", user, cookie)
	return hex.EncodeToString(h.Sum(nil))
}
func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".settings-write-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func readConfig(path string) ([]byte, map[string]json.RawMessage, string, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", 0, err
	}
	fields := map[string]json.RawMessage{}
	if err = json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, nil, "", 0, errors.New("invalid config")
	}
	var token string
	if err = json.Unmarshal(fields["token"], &token); err != nil {
		return nil, nil, "", 0, err
	}
	index := 0
	if value, ok := fields["routeridx"]; ok {
		if err = json.Unmarshal(value, &index); err != nil {
			var text string
			if err = json.Unmarshal(value, &text); err != nil {
				return nil, nil, "", 0, err
			}
			if index, err = strconv.Atoi(text); err != nil {
				return nil, nil, "", 0, err
			}
		}
	}
	return raw, fields, token, index, nil
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	fail := func(code int, message string) { http.Error(w, message, code) }
	user, err := a.authenticate(r)
	if err != nil || user == "" {
		fail(403, "请先使用 DSM 管理员账号登录，再打开 DDNSTO 设置。")
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		w.Header().Set("Allow", "GET, POST")
		fail(405, "不支持此操作")
		return
	}
	// CGI processes share this lock: no lost writes or concurrent restarts.
	lock, err := os.OpenFile(filepath.Join(a.dataDir, ".settings.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		fail(503, "无法访问套件配置目录，请检查套件权限。")
		return
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		fail(409, "正在保存配置，请稍后刷新重试。")
		return
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	keyPath := filepath.Join(a.dataDir, ".settings-key")
	key, err := os.ReadFile(keyPath)
	if os.IsNotExist(err) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err == nil {
			err = atomicWrite(keyPath, key)
		}
	}
	if err != nil || len(key) != 32 {
		fail(503, "无法初始化设置页，请检查套件权限。")
		return
	}
	csrf := signature(key, user, r.Header.Get("Cookie"))
	configPath := filepath.Join(a.dataDir, "ddnsto-config.json")
	original, fields, token, index, err := readConfig(configPath)
	if err != nil {
		fail(503, "无法读取 DDNSTO 配置，请检查配置文件。")
		return
	}
	code, message, failed := 200, "", false
	result := "saved"
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err = r.ParseForm(); err != nil {
			fail(400, "提交内容无效或过长。")
			return
		}
		if !hmac.Equal([]byte(r.PostForm.Get("csrf")), []byte(csrf)) {
			fail(403, "页面校验失败，请刷新后重试。")
			return
		}
		if r.PostForm.Get("revision") != digest(original) {
			fail(409, "配置已在其他页面修改，请刷新后重试。")
			return
		}
		nextToken := strings.TrimSpace(r.PostForm.Get("token"))
		nextIndex, indexErr := strconv.Atoi(r.PostForm.Get("routeridx"))
		if !validToken.MatchString(nextToken) || indexErr != nil || nextIndex < 0 || nextIndex > 99 {
			fail(400, "Token 只能包含字母、数字、下划线和连字符；设备编号必须是 0–99 的整数。")
			return
		}
		running, statusErr := a.control("status")
		if statusErr != nil {
			fail(503, "无法确认服务状态，配置未修改。")
			return
		}
		if nextToken == token && nextIndex == index {
			message = "配置没有变化。"
			result = "unchanged"
		} else {
			fields["token"], _ = json.Marshal(nextToken)
			fields["routeridx"], _ = json.Marshal(nextIndex)
			updated, _ := json.MarshalIndent(fields, "", "  ")
			updated = append(updated, '\n')
			if err = a.save(original, updated, running); err != nil {
				code = 500
				message = err.Error()
				failed = true
			} else {
				original = updated
				token = nextToken
				index = nextIndex
				if running {
					message = "配置已保存，DDNSTO 已重新启动。"
				} else {
					message = "配置已保存。套件当前已停止，请在套件中心启动后生效。"
				}
			}
		}
		if !failed {
			query := r.URL.Query()
			query.Set("result", result)
			target := url.URL{Path: "/" + strings.TrimLeft(r.URL.Path, "/"), RawQuery: query.Encode()}
			http.Redirect(w, r, target.String(), http.StatusSeeOther)
			return
		}
		a.render(w, pageData{User: user, Token: token, Index: index, CSRF: csrf, Revision: digest(original), Running: running, Message: message, Failed: failed}, code)
		return
	}
	running, err := a.control("status")
	if err != nil {
		fail(503, "无法读取套件运行状态，请稍后重试。")
		return
	}
	switch r.URL.Query().Get("result") {
	case "saved":
		if running {
			message = "配置已保存，DDNSTO 已重新启动。"
		} else {
			message = "配置已保存。套件当前已停止，请在套件中心启动后生效。"
		}
	case "unchanged":
		message = "配置没有变化。"
	}
	a.render(w, pageData{User: user, Token: token, Index: index, CSRF: csrf, Revision: digest(original), Running: running, Message: message}, 200)
}

func (a *app) render(w http.ResponseWriter, p pageData, status int) {
	a.diagnostics(&p)
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		http.Error(w, "页面初始化失败", 500)
		return
	}
	p.Nonce = hex.EncodeToString(nonce)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+p.Nonce+"'; style-src 'nonce-"+p.Nonce+"'; img-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'self'")
	w.WriteHeader(status)
	_ = pageTemplate.Execute(w, p)
}

func (a *app) save(original, updated []byte, running bool) error {
	configPath := filepath.Join(a.dataDir, "ddnsto-config.json")
	bootPath := filepath.Join(a.dataDir, ".ddnsto_boot.cfg")
	// Validate cache readability before stopping the service.
	boot, err := os.ReadFile(bootPath)
	hadBoot := err == nil
	if err != nil && !os.IsNotExist(err) {
		return errors.New("无法读取启动缓存，配置未修改。")
	}
	if running {
		if _, err = a.control("stop"); err != nil {
			return errors.New("无法停止客户端，配置未修改。")
		}
	}
	// Capture the latest cache after the process has stopped writing it.
	if b, e := os.ReadFile(bootPath); e == nil {
		boot = b
		hadBoot = true
	}
	rollback := func() error {
		e1 := atomicWrite(configPath, original)
		var e2 error
		if hadBoot {
			e2 = atomicWrite(bootPath, boot)
		} else {
			e2 = os.Remove(bootPath)
			if os.IsNotExist(e2) {
				e2 = nil
			}
		}
		var e3 error
		if running && e1 == nil && e2 == nil {
			_, e3 = a.control("start")
		}
		if e1 != nil || e2 != nil || e3 != nil {
			return errors.New("保存或重启失败，自动恢复未完成。请在套件中心检查服务和配置。")
		}
		return errors.New("保存或重启失败，已恢复原配置，请检查套件日志后重试。")
	}
	if err = atomicWrite(configPath, updated); err != nil {
		return rollback()
	}
	// An invalid primary cache prevents loading another instance's /tmp fallback.
	if err = atomicWrite(bootPath, []byte("0\n")); err != nil {
		return rollback()
	}
	if running {
		if _, err = a.control("start"); err != nil {
			return rollback()
		}
	}
	return nil
}

func authenticateDSM(r *http.Request) (string, error) {
	id := "/usr/bin/id"
	if _, err := os.Stat(id); err != nil {
		id = "/bin/id"
	}
	return authenticateCommands(r, authCGI, id)
}

func authenticateCommands(r *http.Request, authPath, idPath string) (string, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, authPath)
	// Pass only CGI/session metadata, never loader variables or user-selected paths.
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/syno/bin", "REQUEST_METHOD=GET", "CONTENT_LENGTH=0"}
	for _, key := range []string{"HTTP_COOKIE", "HTTP_X_SYNO_TOKEN", "REMOTE_ADDR", "REMOTE_PORT", "SERVER_ADDR", "SERVER_PORT", "SERVER_PROTOCOL", "SERVER_NAME", "HTTP_HOST", "HTTPS", "QUERY_STRING", "REQUEST_URI", "SCRIPT_NAME"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	result, err := cmd.Output()
	user := strings.TrimSpace(string(result))
	if err != nil || user == "" || strings.HasPrefix(user, "-") || strings.ContainsAny(user, "\x00\r\n\t ") {
		return "", errors.New("not authenticated")
	}
	groupsCmd := exec.CommandContext(ctx, idPath, "-Gn", user)
	groupsCmd.Env = []string{"PATH=/usr/bin:/bin"}
	groups, err := groupsCmd.Output()
	if err != nil {
		return "", err
	}
	for _, group := range strings.Fields(string(groups)) {
		if group == "administrators" {
			return user, nil
		}
	}
	return "", errors.New("administrator required")
}

func main() {
	// The DSM tool rule grants setuid to ddnsto, never root. Normalize real IDs
	// before starting the service shell, which would otherwise drop the package UID.
	uid := os.Geteuid()
	if uid == 0 || syscall.Setresuid(uid, uid, uid) != nil {
		fmt.Print("Status: 503 Service Unavailable\r\nContent-Type: text/plain; charset=utf-8\r\nCache-Control: no-store\r\n\r\n设置页需要以 ddnsto 套件用户运行。")
		return
	}
	dataDir := filepath.Join(packageRoot, "var")
	if stat, err := os.Stat(dataDir); err != nil || !stat.IsDir() {
		dataDir = filepath.Join(packageRoot, "etc")
	}
	a := &app{dataDir: dataDir, statusPath: filepath.Join(packageRoot, "target/bin/.ddnsto_http_addr.json"), authenticate: authenticateDSM}
	a.control = func(action string) (bool, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/bin/sh", filepath.Join(packageRoot, "scripts/start-stop-status"), action)
		cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/syno/bin", "SYNOPKG_PKGDEST=" + filepath.Join(packageRoot, "target"), "SYNOPKG_PKGVAR=" + dataDir}
		err := cmd.Run()
		if action == "status" {
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 3 {
				return false, nil
			}
		}
		return err == nil, err
	}
	_ = cgi.Serve(a)
}
