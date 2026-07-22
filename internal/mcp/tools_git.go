package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func gitCmd(ctx context.Context, repo string, args ...string) (*mcp.CallToolResult, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return mcp.NewToolResultError("git not found on PATH"), nil
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	if repo != "" {
		cmd.Dir = repo
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

func (m *Manager) handleGitStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo", "")
	return gitCmd(ctx, repo, "status", "--porcelain=v1", "--branch")
}

func (m *Manager) handleGitDiff(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo", "")
	target := req.GetString("target", "")
	args := []string{"diff"}
	if target != "" {
		args = append(args, target)
	}
	return gitCmd(ctx, repo, args...)
}

func (m *Manager) handleGitLog(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo", "")
	limit := req.GetInt("limit", 20)
	return gitCmd(ctx, repo, "log", fmt.Sprintf("-n%d", limit), "--oneline")
}

func (m *Manager) handleGitBranch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo", "")
	return gitCmd(ctx, repo, "branch", "-a")
}

func (m *Manager) handleGitAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo", "")
	paths := req.GetStringSlice("paths", []string{"."})
	args := append([]string{"add"}, paths...)
	return gitCmd(ctx, repo, args...)
}

func (m *Manager) handleGitCommit(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo", "")
	message, err := req.RequireString("message")
	if err != nil {
		return errInvalidParams("message is required"), nil
	}
	return gitCmd(ctx, repo, "commit", "-m", message)
}

func (m *Manager) handleGitFetch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo", "")
	remote := req.GetString("remote", "")
	args := []string{"fetch"}
	if remote != "" {
		args = append(args, remote)
	}
	return gitCmd(ctx, repo, args...)
}

func (m *Manager) handleGitPull(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo", "")
	remote := req.GetString("remote", "")
	args := []string{"pull"}
	if remote != "" {
		args = append(args, remote)
	}
	return gitCmd(ctx, repo, args...)
}

func (m *Manager) handleGitPush(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo", "")
	remote := req.GetString("remote", "origin")
	branch := req.GetString("branch", "")
	args := []string{"push", remote}
	if branch != "" {
		args = append(args, branch)
	}
	return gitCmd(ctx, repo, args...)
}

func (m *Manager) handleGitHubRepo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return m.githubGET(ctx, "repos/"+req.GetString("repo", ""))
}

func (m *Manager) handleGitHubIssues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo", "")
	state := req.GetString("state", "open")
	return m.githubGET(ctx, fmt.Sprintf("repos/%s/issues?state=%s", repo, state))
}

func (m *Manager) handleGitHubPullRequests(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := req.GetString("repo", "")
	state := req.GetString("state", "open")
	return m.githubGET(ctx, fmt.Sprintf("repos/%s/pulls?state=%s", repo, state))
}

func (m *Manager) githubGET(ctx context.Context, path string) (*mcp.CallToolResult, error) {
	token := strings.TrimSpace(lookupEnv("GITHUB_TOKEN"))
	if token == "" {
		return mcp.NewToolResultError("GITHUB_TOKEN env not set; github tools require a read-scoped token"), nil
	}
	url := "https://api.github.com/" + path
	req, err := newHTTPRequest(ctx, "GET", url, nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("github request failed: %v", err)), nil
	}
	defer resp.Body.Close()
	body, _ := readLimited(resp.Body, maxShellOutput)
	if resp.StatusCode >= 400 {
		return mcp.NewToolResultError(fmt.Sprintf("github %s: %s", resp.Status, string(body))), nil
	}
	var pretty interface{}
	if json.Unmarshal(body, &pretty) == nil {
		if b, err := json.MarshalIndent(pretty, "", "  "); err == nil {
			return mcp.NewToolResultText(string(b)), nil
		}
	}
	return mcp.NewToolResultText(string(body)), nil
}
