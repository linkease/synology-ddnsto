package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var secretField = regexp.MustCompile(`(?i)((?:token|password|authorization|secret)["']?\s*[:=]\s*["']?)[^\s,"'}]+`)

func readLogTail(path, token string) string {
	f, err := os.Open(path)
	if err != nil {
		return "日志暂不可读取（尚未生成或权限不足）。"
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "无法读取日志。"
	}
	offset := info.Size() - 65536
	if offset < 0 {
		offset = 0
	}
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return "无法读取日志。"
	}
	raw, err := io.ReadAll(io.LimitReader(f, 65536))
	if err != nil {
		return "无法读取日志。"
	}
	text := strings.ToValidUTF8(string(raw), "�")
	if offset > 0 {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	text = strings.Join(lines, "\n")
	if token != "" {
		text = strings.ReplaceAll(text, token, "[REDACTED]")
	}
	return secretField.ReplaceAllString(text, "${1}[REDACTED]")
}

func (a *app) diagnostics(p *pageData) {
	p.Connection = "客户端已停止"
	p.DeviceID = "暂不可用（客户端启动并提供状态后显示）"
	p.Version = "未知"
	p.LogPath = filepath.Join(a.dataDir, "logs/ddnstoshell.log")
	p.Logs = "连接日志：\n" + readLogTail(p.LogPath, p.Token) + "\n\n启动输出：\n" + readLogTail(filepath.Join(a.dataDir, "ddnsto.log"), p.Token)
	if !p.Running {
		return
	}
	p.Connection = "进程运行中，连接状态暂不可读取，请查看日志"
	// Discover only this package's endpoint, and reject stale records and redirects.
	var addr struct {
		Addr string `json:"addr"`
		PID  int    `json:"pid"`
	}
	raw, err := os.ReadFile(a.statusPath)
	if err != nil || json.Unmarshal(raw, &addr) != nil {
		return
	}
	pid, err := os.ReadFile(filepath.Join(a.dataDir, "ddnsto.pid"))
	if err != nil || addr.PID <= 1 || strings.TrimSpace(string(pid)) != strconv.Itoa(addr.PID) {
		return
	}
	host, port, err := net.SplitHostPort(addr.Addr)
	if err != nil || host != "127.0.0.1" {
		return
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Get("http://" + addr.Addr + "/status")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	var status struct {
		Connected *bool  `json:"connected"`
		RouterID  string `json:"routerId"`
		Version   string `json:"version"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&status) != nil || status.Connected == nil {
		return
	}
	p.Connection = "未连接服务端，请查看日志"
	if *status.Connected {
		p.Connection = "已连接服务端"
	}
	if status.RouterID != "" {
		p.DeviceID = status.RouterID
	}
	if status.Version != "" {
		p.Version = status.Version
	}
}
