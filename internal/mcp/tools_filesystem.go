package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	maxFileBytes   = 4 * 1024 * 1024
	defaultDataDir = "data"
)

// allowedRoots is the configured set of filesystem roots tools may access.
var allowedRoots = []string{}

// SetAllowedRoots configures the filesystem roots for fs.* tools.
func SetAllowedRoots(roots []string) {
	if len(roots) == 0 {
		allowedRoots = []string{defaultDataDir}
		return
	}
	allowedRoots = roots
}

// resolveSafePath ensures p is contained within an allowed root. It rejects
// traversal outside roots, symlink/junction escapes and UNC/device paths.
func resolveSafePath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	clean := filepath.Clean(p)
	if strings.HasPrefix(clean, `\\`) || strings.HasPrefix(clean, `\\?\`) {
		return "", fmt.Errorf("UNC/device paths are not allowed: %q", p)
	}
	for _, root := range allowedRoots {
		r := filepath.Clean(root)
		if r == clean {
			return clean, nil
		}
		if strings.HasPrefix(clean, r+string(os.PathSeparator)) {
			if resolved, err := filepath.EvalSymlinks(clean); err == nil {
				if !strings.HasPrefix(resolved, r+string(os.PathSeparator)) && resolved != r {
					return "", fmt.Errorf("path escapes allowed root via symlink: %q", p)
				}
				return resolved, nil
			}
			return clean, nil
		}
	}
	return "", fmt.Errorf("path %q is outside allowed roots", p)
}

func (m *Manager) handleFSReadFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathArg, err := req.RequireString("path")
	if err != nil {
		return errInvalidParams("path is required"), nil
	}
	resolved, err := resolveSafePath(pathArg)
	if err != nil {
		return errInvalidParams(err.Error()), nil
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("read failed: %v", err)), nil
	}
	if len(data) > maxFileBytes {
		return mcp.NewToolResultError("file exceeds max read size (4MB)"), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (m *Manager) handleFSReadText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return m.handleFSReadFile(ctx, req)
}

func (m *Manager) handleFSStat(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathArg, err := req.RequireString("path")
	if err != nil {
		return errInvalidParams("path is required"), nil
	}
	resolved, err := resolveSafePath(pathArg)
	if err != nil {
		return errInvalidParams(err.Error()), nil
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("stat failed: %v", err)), nil
	}
	out := map[string]interface{}{
		"name":    info.Name(),
		"size":    info.Size(),
		"mode":    info.Mode().String(),
		"modtime": info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
		"is_dir":  info.IsDir(),
	}
	b, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleFSList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathArg, err := req.RequireString("path")
	if err != nil {
		return errInvalidParams("path is required"), nil
	}
	resolved, err := resolveSafePath(pathArg)
	if err != nil {
		return errInvalidParams(err.Error()), nil
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list failed: %v", err)), nil
	}
	out := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]interface{}{"name": e.Name(), "is_dir": e.IsDir()})
	}
	b, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleFSSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pattern, err := req.RequireString("pattern")
	if err != nil {
		return errInvalidParams("pattern is required"), nil
	}
	root := req.GetString("root", "")
	resolvedRoot := "."
	if root != "" {
		resolvedRoot, err = resolveSafePath(root)
		if err != nil {
			return errInvalidParams(err.Error()), nil
		}
	} else if len(allowedRoots) > 0 {
		resolvedRoot = allowedRoots[0]
	}
	limit := req.GetInt("limit", 100)
	var matches []string
	_ = filepath.WalkDir(resolvedRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(matches) >= limit {
			return filepath.SkipAll
		}
		if strings.Contains(filepath.Base(p), pattern) {
			matches = append(matches, p)
		}
		return nil
	})
	b, _ := json.Marshal(matches)
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleFSWriteFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathArg, err := req.RequireString("path")
	if err != nil {
		return errInvalidParams("path is required"), nil
	}
	content, err := req.RequireString("content")
	if err != nil {
		return errInvalidParams("content is required"), nil
	}
	resolved, err := resolveSafePath(pathArg)
	if err != nil {
		return errInvalidParams(err.Error()), nil
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o700); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("mkdir failed: %v", err)), nil
	}
	if err := os.WriteFile(resolved, []byte(content), 0o600); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("write failed: %v", err)), nil
	}
	return mcp.NewToolResultText("ok"), nil
}

func (m *Manager) handleFSAppendFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathArg, err := req.RequireString("path")
	if err != nil {
		return errInvalidParams("path is required"), nil
	}
	content, err := req.RequireString("content")
	if err != nil {
		return errInvalidParams("content is required"), nil
	}
	resolved, err := resolveSafePath(pathArg)
	if err != nil {
		return errInvalidParams(err.Error()), nil
	}
	f, err := os.OpenFile(resolved, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("open failed: %v", err)), nil
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("append failed: %v", err)), nil
	}
	return mcp.NewToolResultText("ok"), nil
}

func (m *Manager) handleFSMkdir(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathArg, err := req.RequireString("path")
	if err != nil {
		return errInvalidParams("path is required"), nil
	}
	resolved, err := resolveSafePath(pathArg)
	if err != nil {
		return errInvalidParams(err.Error()), nil
	}
	if err := os.MkdirAll(resolved, 0o700); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("mkdir failed: %v", err)), nil
	}
	return mcp.NewToolResultText("ok"), nil
}

func (m *Manager) handleFSCopy(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	src, err := req.RequireString("src")
	if err != nil {
		return errInvalidParams("src is required"), nil
	}
	dst, err := req.RequireString("dst")
	if err != nil {
		return errInvalidParams("dst is required"), nil
	}
	rs, err := resolveSafePath(src)
	if err != nil {
		return errInvalidParams("src: " + err.Error()), nil
	}
	rd, err := resolveSafePath(dst)
	if err != nil {
		return errInvalidParams("dst: " + err.Error()), nil
	}
	if err := copyFile(rs, rd); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("copy failed: %v", err)), nil
	}
	return mcp.NewToolResultText("ok"), nil
}

func (m *Manager) handleFSMove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	src, err := req.RequireString("src")
	if err != nil {
		return errInvalidParams("src is required"), nil
	}
	dst, err := req.RequireString("dst")
	if err != nil {
		return errInvalidParams("dst is required"), nil
	}
	rs, err := resolveSafePath(src)
	if err != nil {
		return errInvalidParams("src: " + err.Error()), nil
	}
	rd, err := resolveSafePath(dst)
	if err != nil {
		return errInvalidParams("dst: " + err.Error()), nil
	}
	if err := os.Rename(rs, rd); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("move failed: %v", err)), nil
	}
	return mcp.NewToolResultText("ok"), nil
}

func (m *Manager) handleFSRename(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return m.handleFSMove(ctx, req)
}

func (m *Manager) handleFSDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathArg, err := req.RequireString("path")
	if err != nil {
		return errInvalidParams("path is required"), nil
	}
	resolved, err := resolveSafePath(pathArg)
	if err != nil {
		return errInvalidParams(err.Error()), nil
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("stat failed: %v", err)), nil
	}
	if info.IsDir() {
		if !req.GetBool("recursive", false) {
			return mcp.NewToolResultError("refusing to delete directory without recursive=true"), nil
		}
		if err := os.RemoveAll(resolved); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("rmdir failed: %v", err)), nil
		}
	} else {
		if err := os.Remove(resolved); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete failed: %v", err)), nil
		}
	}
	return mcp.NewToolResultText("ok"), nil
}

func (m *Manager) handleFSHash(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathArg, err := req.RequireString("path")
	if err != nil {
		return errInvalidParams("path is required"), nil
	}
	resolved, err := resolveSafePath(pathArg)
	if err != nil {
		return errInvalidParams(err.Error()), nil
	}
	f, err := os.Open(resolved)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("open failed: %v", err)), nil
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("hash failed: %v", err)), nil
	}
	return mcp.NewToolResultText(hex.EncodeToString(h.Sum(nil))), nil
}

func (m *Manager) handleFSBatch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ops, err := req.RequireStringSlice("ops")
	if err != nil {
		return errInvalidParams("ops (list of paths to hash) is required"), nil
	}
	results := make([]map[string]string, 0, len(ops))
	for _, p := range ops {
		resolved, err := resolveSafePath(p)
		if err != nil {
			results = append(results, map[string]string{"path": p, "error": err.Error()})
			continue
		}
		f, err := os.Open(resolved)
		if err != nil {
			results = append(results, map[string]string{"path": p, "error": err.Error()})
			continue
		}
		h := sha256.New()
		_, _ = io.Copy(h, f)
		f.Close()
		results = append(results, map[string]string{"path": p, "sha256": hex.EncodeToString(h.Sum(nil))})
	}
	b, _ := json.Marshal(results)
	return mcp.NewToolResultText(string(b)), nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
