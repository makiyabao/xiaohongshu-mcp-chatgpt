// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package downloader

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/h2non/filetype"
)

const (
	maxChatGPTImageBytes = 15 << 20
	chatGPTUploadTTL     = 2 * time.Hour
)

// ChatGPTFile is the file object passed by ChatGPT through openai/fileParams.
// Do not log these values: download_url is temporary and file_id is an opaque reference.
type ChatGPTFile struct {
	DownloadURL string `json:"download_url" jsonschema:"ChatGPT提供的临时HTTPS下载地址"`
	FileID      string `json:"file_id" jsonschema:"ChatGPT文件引用，不用于日志或对外返回"`
	MIMEType    string `json:"mime_type,omitempty" jsonschema:"文件声明的MIME类型，仅作提示"`
	FileName    string `json:"file_name,omitempty" jsonschema:"原始文件名，仅作提示"`
}

type stagedImage struct {
	path      string
	expiresAt time.Time
}

// StagedImageStore maps unguessable, expiring handles to server-generated paths.
// Callers can never submit a path to this store.
type StagedImageStore struct {
	dir   string
	mu    sync.Mutex
	items map[string]stagedImage
}

func NewStagedImageStore(dir string) *StagedImageStore {
	s := &StagedImageStore{dir: dir, items: make(map[string]stagedImage)}
	s.cleanup()
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.cleanup()
		}
	}()
	return s
}

func (s *StagedImageStore) Import(ctx context.Context, input ChatGPTFile) (string, error) {
	path, err := DownloadChatGPTImage(ctx, input, s.dir)
	if err != nil {
		return "", err
	}
	var randomToken [24]byte
	if _, err := rand.Read(randomToken[:]); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("生成图片句柄失败")
	}
	token := "img_" + hex.EncodeToString(randomToken[:])
	s.mu.Lock()
	s.items[token] = stagedImage{path: path, expiresAt: time.Now().Add(chatGPTUploadTTL)}
	s.mu.Unlock()
	return token, nil
}

func (s *StagedImageStore) Resolve(tokens []string) ([]string, error) {
	if len(tokens) == 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	paths := make([]string, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if _, duplicate := seen[token]; duplicate {
			return nil, fmt.Errorf("图片句柄重复")
		}
		seen[token] = struct{}{}
		entry, ok := s.items[token]
		if !ok || time.Now().After(entry.expiresAt) {
			return nil, fmt.Errorf("图片句柄不存在或已过期")
		}
		info, err := os.Stat(entry.path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("暂存图片不可访问")
		}
		file, err := os.Open(entry.path)
		if err != nil {
			return nil, fmt.Errorf("暂存图片不可读")
		}
		_ = file.Close()
		paths = append(paths, entry.path)
	}
	return paths, nil
}

func (s *StagedImageStore) Delete(tokens []string) {
	var paths []string
	s.mu.Lock()
	for _, token := range tokens {
		if entry, ok := s.items[token]; ok {
			paths = append(paths, entry.path)
			delete(s.items, token)
		}
	}
	s.mu.Unlock()
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func (s *StagedImageStore) cleanup() {
	now := time.Now()
	var expired []string
	s.mu.Lock()
	for token, entry := range s.items {
		if now.After(entry.expiresAt) {
			expired = append(expired, entry.path)
			delete(s.items, token)
		}
	}
	s.mu.Unlock()
	for _, path := range expired {
		_ = os.Remove(path)
	}
	cleanupChatGPTUploads(s.dir)
}

// DownloadChatGPTImage fetches a ChatGPT file reference into a private, randomized staging path.
func DownloadChatGPTImage(ctx context.Context, input ChatGPTFile, dir string) (string, error) {
	if strings.TrimSpace(input.FileID) == "" {
		return "", fmt.Errorf("ChatGPT 文件引用缺少 file_id")
	}
	if err := validatePublicHTTPSURL(ctx, input.DownloadURL); err != nil {
		return "", fmt.Errorf("ChatGPT 图片下载地址无效")
	}
	if input.MIMEType != "" && !strings.HasPrefix(strings.ToLower(input.MIMEType), "image/") {
		return "", fmt.Errorf("附件声明的类型不是图片")
	}

	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 12 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			var lastErr error
			for _, candidate := range ips {
				if !isPublicIP(candidate.IP) {
					continue
				}
				conn, dialErr := (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				lastErr = dialErr
			}
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, fmt.Errorf("host resolved only to non-public addresses")
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   25 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return validatePublicHTTPSURL(req.Context(), req.URL.String())
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.DownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建图片下载请求失败")
	}
	req.Header.Set("Accept", "image/*")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载 ChatGPT 图片失败，网络或临时链接异常")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载 ChatGPT 图片失败，HTTP 状态码 %d", resp.StatusCode)
	}
	if resp.ContentLength > maxChatGPTImageBytes {
		return "", fmt.Errorf("图片超过 15 MiB 大小限制")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxChatGPTImageBytes+1))
	if err != nil {
		return "", fmt.Errorf("读取 ChatGPT 图片失败")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("下载到的图片为空")
	}
	if len(data) > maxChatGPTImageBytes {
		return "", fmt.Errorf("图片超过 15 MiB 大小限制")
	}
	kind, err := filetype.Match(data)
	if err != nil || !filetype.IsImage(data) {
		return "", fmt.Errorf("附件内容不是有效图片")
	}
	mimeType := strings.ToLower(kind.MIME.Value)
	switch mimeType {
	case "image/jpeg", "image/png", "image/webp", "image/gif", "image/heic", "image/heif":
	default:
		return "", fmt.Errorf("不支持的图片格式")
	}
	ext := strings.TrimPrefix(strings.ToLower(kind.Extension), ".")
	if ext == "" || strings.ContainsAny(ext, `/\\`) {
		return "", fmt.Errorf("无法识别图片扩展名")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("创建图片暂存目录失败")
	}
	_ = os.Chmod(dir, 0700)
	var randomName [16]byte
	if _, err := rand.Read(randomName[:]); err != nil {
		return "", fmt.Errorf("生成图片暂存名失败")
	}
	path := filepath.Join(dir, "chatgpt_"+hex.EncodeToString(randomName[:])+"."+ext)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", fmt.Errorf("创建图片暂存文件失败")
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("保存图片暂存文件失败")
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("关闭图片暂存文件失败")
	}
	return path, nil
}

func validatePublicHTTPSURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return fmt.Errorf("HTTPS URL required")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "metadata.google.internal" {
		return fmt.Errorf("non-public hostname")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return fmt.Errorf("non-public address")
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("hostname resolution failed")
	}
	for _, candidate := range ips {
		if !isPublicIP(candidate.IP) {
			return fmt.Errorf("hostname resolves to a non-public address")
		}
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	// Shared address space (RFC 6598) is not a public destination either.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	return true
}

func cleanupChatGPTUploads(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-chatGPTUploadTTL)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "chatgpt_") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
}
