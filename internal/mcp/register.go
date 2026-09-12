package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/ti/router/tibrain/internal/security"
)

// strSlice builds an array-of-strings tool property (returns a ToolOption so it
// can be passed directly to mcp.NewTool).
func strSlice(name string, required bool, desc string) mcp.ToolOption {
	if required {
		return mcp.WithArray(name, mcp.Required(), mcp.Description(desc), mcp.WithStringItems())
	}
	return mcp.WithArray(name, mcp.Description(desc), mcp.WithStringItems())
}

// addTool registers a tool into the registry and the underlying mcp-go server,
// wrapping the handler with permission + audit enforcement.
func (m *Manager) addTool(name, desc string, cat security.Category, tool mcp.Tool, handler server.ToolHandlerFunc) {
	rec := ToolRecord{Name: name, Description: desc, Category: cat, Tool: tool, Handler: handler}
	m.registry.Register(rec)
	m.srv.AddTool(tool, m.wrapGuard(rec, handler))
}

// registerAllTools wires every real, implemented tool. Unimplemented tools are
// intentionally NOT registered (no placeholder/mock success).
func (m *Manager) registerAllTools() {
	// ---- Memory (knowledge retrieval) ----
	m.addTool("memory.search", "Search shared knowledge entries from memory_index.yaml with confidence filtering.", security.CatRead,
		mcp.NewTool("memory.search",
			mcp.WithDescription("Query shared memory knowledge base with domain + confidence-aware filtering"),
			mcp.WithString("query", mcp.Required(), mcp.Description("Search term to match entries")),
			mcp.WithNumber("min_confidence", mcp.Description("Minimum confidence score (0.0-1.0), default 0.8")),
			mcp.WithString("domain", mcp.Description("Optional: filter to specific domain (github_auth, git_workflow, go_patterns, tibrain_arch, dev_environment)")),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10)")),
		), m.handleMemorySearch)

	m.addTool("memory.list_domains", "List available memory domains with confidence scores.", security.CatRead,
		mcp.NewTool("memory.list_domains",
			mcp.WithDescription("List all indexed knowledge domains from memory_index.yaml"),
		), m.handleMemoryListDomains)

	// ---- Filesystem (read) ----
	m.addTool("fs.read_file", "Read a file within an allowed root. Read-only, side-effect free.", security.CatRead,
		mcp.NewTool("fs.read_file",
			mcp.WithDescription("Read a file within an allowed root. Read-only, side-effect free."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Absolute path under an allowed root")),
		), m.handleFSReadFile)

	m.addTool("fs.read_text", "Alias of fs.read_file. Read-only.", security.CatRead,
		mcp.NewTool("fs.read_text",
			mcp.WithDescription("Read a UTF-8 text file within an allowed root."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Absolute path under an allowed root")),
		), m.handleFSReadText)

	m.addTool("fs.stat", "Return metadata for a path. Read-only.", security.CatRead,
		mcp.NewTool("fs.stat",
			mcp.WithDescription("Stat a file/dir within an allowed root."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Absolute path under an allowed root")),
		), m.handleFSStat)

	m.addTool("fs.list", "List a directory. Read-only.", security.CatRead,
		mcp.NewTool("fs.list",
			mcp.WithDescription("List directory entries within an allowed root."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Directory path under an allowed root")),
		), m.handleFSList)

	m.addTool("fs.search", "Search filenames by substring within an allowed root. Read-only.", security.CatRead,
		mcp.NewTool("fs.search",
			mcp.WithDescription("Search for files by name substring."),
			mcp.WithString("pattern", mcp.Required(), mcp.Description("Substring to match in file names")),
			mcp.WithString("root", mcp.Description("Optional root to search under")),
			mcp.WithNumber("limit", mcp.Description("Max matches (default 100)")),
		), m.handleFSSearch)

	// ---- Filesystem (write) ----
	m.addTool("fs.write_file", "Write content to a file. Creates parent dirs. Side-effect: overwrites.", security.CatWrite,
		mcp.NewTool("fs.write_file",
			mcp.WithDescription("Write content to a file (overwrites). Creates parent directories."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Absolute path under an allowed root")),
			mcp.WithString("content", mcp.Required(), mcp.Description("File content")),
		), m.handleFSWriteFile)

	m.addTool("fs.append_file", "Append content to a file. Side-effect.", security.CatWrite,
		mcp.NewTool("fs.append_file",
			mcp.WithDescription("Append content to an existing or new file."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Absolute path under an allowed root")),
			mcp.WithString("content", mcp.Required(), mcp.Description("Content to append")),
		), m.handleFSAppendFile)

	m.addTool("fs.mkdir", "Create a directory tree. Side-effect.", security.CatWrite,
		mcp.NewTool("fs.mkdir",
			mcp.WithDescription("Create a directory and parents."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Directory path under an allowed root")),
		), m.handleFSMkdir)

	m.addTool("fs.copy", "Copy a file. Side-effect.", security.CatWrite,
		mcp.NewTool("fs.copy",
			mcp.WithDescription("Copy a file to a destination under an allowed root."),
			mcp.WithString("src", mcp.Required(), mcp.Description("Source path")),
			mcp.WithString("dst", mcp.Required(), mcp.Description("Destination path")),
		), m.handleFSCopy)

	m.addTool("fs.move", "Move/rename a file. Side-effect.", security.CatWrite,
		mcp.NewTool("fs.move",
			mcp.WithDescription("Move or rename a file."),
			mcp.WithString("src", mcp.Required(), mcp.Description("Source path")),
			mcp.WithString("dst", mcp.Required(), mcp.Description("Destination path")),
		), m.handleFSMove)

	m.addTool("fs.rename", "Rename a file (alias of move). Side-effect.", security.CatWrite,
		mcp.NewTool("fs.rename",
			mcp.WithDescription("Rename a file."),
			mcp.WithString("src", mcp.Required(), mcp.Description("Source path")),
			mcp.WithString("dst", mcp.Required(), mcp.Description("Destination path")),
		), m.handleFSRename)

	m.addTool("fs.delete", "Delete a file or (recursive) directory. DESTRUCTIVE.", security.CatDestruct,
		mcp.NewTool("fs.delete",
			mcp.WithDescription("Delete a file or directory. Destructive; requires recursive=true for directories."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Path to delete")),
			mcp.WithBoolean("recursive", mcp.Description("Required true to delete a directory")),
		), m.handleFSDelete)

	m.addTool("fs.hash", "SHA-256 hash of a file. Read-only.", security.CatRead,
		mcp.NewTool("fs.hash",
			mcp.WithDescription("Compute SHA-256 of a file."),
			mcp.WithString("path", mcp.Required(), mcp.Description("File path")),
		), m.handleFSHash)

	m.addTool("fs.batch", "Batch hash a list of files. Read-only.", security.CatRead,
		mcp.NewTool("fs.batch",
			mcp.WithDescription("Hash multiple files; returns per-file sha256 or error."),
			strSlice("ops", true, "List of file paths"),
		), m.handleFSBatch)

	// ---- Shell ----
	m.addTool("shell.exec", "Execute a command with args. Side-effect; returns stdout/stderr/exit/duration.", security.CatWrite,
		mcp.NewTool("shell.exec",
			mcp.WithDescription("Run a command with structured args, timeout and output cap."),
			mcp.WithString("command", mcp.Required(), mcp.Description("Executable")),
			strSlice("args", false, "Arguments"),
			mcp.WithNumber("timeout_ms", mcp.Description("Timeout in ms (default 300000)")),
		), m.handleShellExec)

	m.addTool("powershell.exec", "Run a PowerShell script. Side-effect.", security.CatWrite,
		mcp.NewTool("powershell.exec",
			mcp.WithDescription("Run a PowerShell script with timeout and output cap."),
			mcp.WithString("script", mcp.Required(), mcp.Description("PowerShell script")),
			mcp.WithNumber("timeout_ms", mcp.Description("Timeout in ms")),
		), m.handlePowershellExec)

	m.addTool("shell.stream", "Start a long-running process, return PID. Side-effect.", security.CatWrite,
		mcp.NewTool("shell.stream",
			mcp.WithDescription("Start a background process; returns its PID."),
			mcp.WithString("command", mcp.Required(), mcp.Description("Executable")),
			strSlice("args", false, "Arguments"),
		), m.handleShellStream)

	m.addTool("shell.cancel", "Kill a managed process by PID. DESTRUCTIVE.", security.CatDestruct,
		mcp.NewTool("shell.cancel",
			mcp.WithDescription("Terminate a process started via shell.stream."),
			mcp.WithNumber("pid", mcp.Required(), mcp.Description("Process ID")),
		), m.handleShellCancel)

	// ---- Process / service / network ----
	m.addTool("process.list", "List running processes. Read-only.", security.CatRead,
		mcp.NewTool("process.list", mcp.WithDescription("List processes (Windows tasklist).")), m.handleProcessList)

	m.addTool("process.inspect", "Inspect a process by PID. Read-only.", security.CatRead,
		mcp.NewTool("process.inspect",
			mcp.WithDescription("Check whether a PID is running."),
			mcp.WithNumber("pid", mcp.Required(), mcp.Description("Process ID")),
		), m.handleProcessInspect)

	m.addTool("process.start", "Start a process. Side-effect.", security.CatWrite,
		mcp.NewTool("process.start",
			mcp.WithDescription("Start a process; returns its PID."),
			mcp.WithString("command", mcp.Required(), mcp.Description("Executable")),
			strSlice("args", false, "Arguments"),
		), m.handleProcessStart)

	m.addTool("process.stop", "Stop a process by PID (verifies it exists first). DESTRUCTIVE.", security.CatDestruct,
		mcp.NewTool("process.stop",
			mcp.WithDescription("Terminate a process by PID. Verifies existence to avoid PID reuse."),
			mcp.WithNumber("pid", mcp.Required(), mcp.Description("Process ID")),
		), m.handleProcessStop)

	m.addTool("service.status", "Query Windows service status. Read-only.", security.CatRead,
		mcp.NewTool("service.status",
			mcp.WithDescription("Query a Windows service via sc."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Service name")),
		), m.handleServiceStatus)

	m.addTool("service.start", "Start a Windows service. DESTRUCTIVE/side-effect.", security.CatDestruct,
		mcp.NewTool("service.start",
			mcp.WithDescription("Start a Windows service."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Service name")),
		), m.handleServiceStart)

	m.addTool("service.stop", "Stop a Windows service. DESTRUCTIVE.", security.CatDestruct,
		mcp.NewTool("service.stop",
			mcp.WithDescription("Stop a Windows service."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Service name")),
		), m.handleServiceStop)

	m.addTool("service.restart", "Restart a Windows service. DESTRUCTIVE.", security.CatDestruct,
		mcp.NewTool("service.restart",
			mcp.WithDescription("Stop then start a Windows service."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Service name")),
		), m.handleServiceRestart)

	m.addTool("network.ports", "List network ports/connections. Read-only.", security.CatRead,
		mcp.NewTool("network.ports", mcp.WithDescription("List active network connections (netstat -ano).")), m.handleNetworkPorts)

	m.addTool("network.connections", "Alias of network.ports. Read-only.", security.CatRead,
		mcp.NewTool("network.connections", mcp.WithDescription("List active network connections.")), m.handleNetworkConnections)

	// ---- Git / GitHub ----
	m.addTool("git.status", "Show git working tree status. Read-only.", security.CatRead,
		mcp.NewTool("git.status",
			mcp.WithDescription("git status (porcelain, with branch)."),
			mcp.WithString("repo", mcp.Description("Repo path (optional)")),
		), m.handleGitStatus)

	m.addTool("git.diff", "Show git diff. Read-only.", security.CatRead,
		mcp.NewTool("git.diff",
			mcp.WithDescription("git diff for a target."),
			mcp.WithString("repo", mcp.Description("Repo path")),
			mcp.WithString("target", mcp.Description("Diff target (optional)")),
		), m.handleGitDiff)

	m.addTool("git.log", "Show git log. Read-only.", security.CatRead,
		mcp.NewTool("git.log",
			mcp.WithDescription("git log (oneline, limited)."),
			mcp.WithString("repo", mcp.Description("Repo path")),
			mcp.WithNumber("limit", mcp.Description("Max entries (default 20)")),
		), m.handleGitLog)

	m.addTool("git.branch", "List git branches. Read-only.", security.CatRead,
		mcp.NewTool("git.branch",
			mcp.WithDescription("git branch -a."),
			mcp.WithString("repo", mcp.Description("Repo path")),
		), m.handleGitBranch)

	m.addTool("git.add", "Stage files. Side-effect.", security.CatWrite,
		mcp.NewTool("git.add",
			mcp.WithDescription("git add paths (default '.')."),
			mcp.WithString("repo", mcp.Description("Repo path")),
			strSlice("paths", false, "Paths to add"),
		), m.handleGitAdd)

	m.addTool("git.commit", "Commit staged changes. Side-effect.", security.CatWrite,
		mcp.NewTool("git.commit",
			mcp.WithDescription("git commit -m message."),
			mcp.WithString("repo", mcp.Description("Repo path")),
			mcp.WithString("message", mcp.Required(), mcp.Description("Commit message")),
		), m.handleGitCommit)

	m.addTool("git.fetch", "Fetch from remote. Side-effect (network).", security.CatWrite,
		mcp.NewTool("git.fetch",
			mcp.WithDescription("git fetch [remote]."),
			mcp.WithString("repo", mcp.Description("Repo path")),
			mcp.WithString("remote", mcp.Description("Remote name")),
		), m.handleGitFetch)

	m.addTool("git.pull", "Pull from remote. Side-effect (network).", security.CatWrite,
		mcp.NewTool("git.pull",
			mcp.WithDescription("git pull [remote]."),
			mcp.WithString("repo", mcp.Description("Repo path")),
			mcp.WithString("remote", mcp.Description("Remote name")),
		), m.handleGitPull)

	m.addTool("git.push", "Push to remote (no force). Side-effect (network).", security.CatWrite,
		mcp.NewTool("git.push",
			mcp.WithDescription("git push origin [branch] (never force)."),
			mcp.WithString("repo", mcp.Description("Repo path")),
			mcp.WithString("remote", mcp.Description("Remote (default origin)")),
			mcp.WithString("branch", mcp.Description("Branch")),
		), m.handleGitPush)

	m.addTool("github.repo", "Get GitHub repo metadata (read-only, token from env). Read-only.", security.CatRead,
		mcp.NewTool("github.repo",
			mcp.WithDescription("GET /repos/{repo} via GitHub API (requires GITHUB_TOKEN)."),
			mcp.WithString("repo", mcp.Required(), mcp.Description("owner/name")),
		), m.handleGitHubRepo)

	m.addTool("github.issues", "List GitHub issues (read-only). Read-only.", security.CatRead,
		mcp.NewTool("github.issues",
			mcp.WithDescription("List issues for a repo (requires GITHUB_TOKEN)."),
			mcp.WithString("repo", mcp.Required(), mcp.Description("owner/name")),
			mcp.WithString("state", mcp.Description("open|closed|all (default open)")),
		), m.handleGitHubIssues)

	m.addTool("github.pull_requests", "List GitHub PRs (read-only). Read-only.", security.CatRead,
		mcp.NewTool("github.pull_requests",
			mcp.WithDescription("List PRs for a repo (requires GITHUB_TOKEN)."),
			mcp.WithString("repo", mcp.Required(), mcp.Description("owner/name")),
			mcp.WithString("state", mcp.Description("open|closed|all (default open)")),
		), m.handleGitHubPullRequests)

	// ---- HTTP ----
	m.addTool("http.request", "Make an HTTP request (SSRF-guarded). Side-effect (network).", security.CatWrite,
		mcp.NewTool("http.request",
			mcp.WithDescription("HTTP request with SSRF guard; headers via header_<name>."),
			mcp.WithString("url", mcp.Required(), mcp.Description("Target URL")),
			mcp.WithString("method", mcp.Description("HTTP method (default GET)")),
			mcp.WithString("body", mcp.Description("Request body")),
			strSlice("allowed_hosts", false, "Host allowlist (bypasses SSRF block)"),
		), m.handleHTTPRequest)

	m.addTool("http.download", "Download a URL to an allowed path (SSRF-guarded). Side-effect.", security.CatWrite,
		mcp.NewTool("http.download",
			mcp.WithDescription("Download a URL into an allowed root."),
			mcp.WithString("url", mcp.Required(), mcp.Description("Source URL")),
			mcp.WithString("dest", mcp.Required(), mcp.Description("Destination path under allowed root")),
			strSlice("allowed_hosts", false, "Host allowlist"),
		), m.handleHTTPDownload)

	m.addTool("http.health_check", "HTTP health probe (SSRF-guarded). Read-only.", security.CatRead,
		mcp.NewTool("http.health_check",
			mcp.WithDescription("Probe a URL and report healthy/unhealthy."),
			mcp.WithString("url", mcp.Required(), mcp.Description("Target URL")),
			strSlice("allowed_hosts", false, "Host allowlist"),
		), m.handleHTTPHealthCheck)

	// ---- Database ----
	m.addTool("db.query", "Run a read-only SQL query. Read-only.", security.CatRead,
		mcp.NewTool("db.query",
			mcp.WithDescription("Execute a SELECT/PRAGMA query against a configured database."),
			mcp.WithString("connection", mcp.Required(), mcp.Description("Connection name (e.g. sqlite)")),
			mcp.WithString("query", mcp.Required(), mcp.Description("SQL query (read-only)")),
		), m.handleDBQuery)

	m.addTool("db.execute", "Run a mutating SQL statement. Side-effect.", security.CatWrite,
		mcp.NewTool("db.execute",
			mcp.WithDescription("Execute a non-query SQL statement."),
			mcp.WithString("connection", mcp.Required(), mcp.Description("Connection name")),
			mcp.WithString("statement", mcp.Required(), mcp.Description("SQL statement")),
		), m.handleDBExecute)

	m.addTool("db.schema", "List tables/views. Read-only.", security.CatRead,
		mcp.NewTool("db.schema",
			mcp.WithDescription("List schema objects for a database."),
			mcp.WithString("connection", mcp.Required(), mcp.Description("Connection name")),
		), m.handleDBSchema)

	m.addTool("db.transaction", "Run statements in a transaction. Side-effect.", security.CatWrite,
		mcp.NewTool("db.transaction",
			mcp.WithDescription("Execute multiple statements atomically."),
			mcp.WithString("connection", mcp.Required(), mcp.Description("Connection name")),
			strSlice("statements", true, "SQL statements"),
		), m.handleDBTransaction)

	// ---- Browser ----
	m.addTool("browser.open", "Open an isolated headless browser session. Side-effect.", security.CatWrite,
		mcp.NewTool("browser.open",
			mcp.WithDescription("Open a headless browser session with an isolated profile."),
			mcp.WithString("url", mcp.Required(), mcp.Description("URL to open")),
		), m.handleBrowserOpen)

	m.addTool("browser.snapshot", "Dump DOM of a browser session. Read-only.", security.CatRead,
		mcp.NewTool("browser.snapshot",
			mcp.WithDescription("Return the rendered DOM via headless dump-dom."),
			mcp.WithString("session", mcp.Required(), mcp.Description("Browser session id")),
		), m.handleBrowserSnapshot)

	m.addTool("browser.extract", "Extract DOM content from a session. Read-only.", security.CatRead,
		mcp.NewTool("browser.extract",
			mcp.WithDescription("Return extractable DOM content."),
			mcp.WithString("session", mcp.Required(), mcp.Description("Browser session id")),
		), m.handleBrowserExtract)

	m.addTool("browser.click", "Click an element (requires CDP). BLOCKED in this build.", security.CatDestruct,
		mcp.NewTool("browser.click",
			mcp.WithDescription("Click an element. Requires chrome-devtools-mcp CDP integration."),
			mcp.WithString("session", mcp.Description("Browser session id")),
			mcp.WithString("selector", mcp.Description("CSS selector")),
		), m.handleBrowserClick)

	m.addTool("browser.fill", "Fill a field (requires CDP). BLOCKED in this build.", security.CatWrite,
		mcp.NewTool("browser.fill",
			mcp.WithDescription("Fill a field. Requires chrome-devtools-mcp CDP integration."),
			mcp.WithString("session", mcp.Description("Browser session id")),
			mcp.WithString("selector", mcp.Description("CSS selector")),
			mcp.WithString("value", mcp.Description("Value")),
		), m.handleBrowserFill)

	m.addTool("browser.close", "Close a browser session and clean up. DESTRUCTIVE.", security.CatDestruct,
		mcp.NewTool("browser.close",
			mcp.WithDescription("Close a browser session and remove its isolated profile."),
			mcp.WithString("session", mcp.Required(), mcp.Description("Browser session id")),
		), m.handleBrowserClose)

	// ---- Workflow / tasks ----
	m.addTool("workflow.create", "Create a persistent workflow. Side-effect.", security.CatWrite,
		mcp.NewTool("workflow.create",
			mcp.WithDescription("Create a workflow with ordered shell steps and optional idempotency key."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Workflow id")),
			strSlice("steps", true, "Shell step commands"),
			mcp.WithString("idempotency_key", mcp.Description("Idempotency key")),
		), m.handleWorkflowCreate)

	m.addTool("workflow.run", "Run a workflow (idempotent steps, retry/backoff). Side-effect.", security.CatWrite,
		mcp.NewTool("workflow.run",
			mcp.WithDescription("Execute a workflow's pending steps."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Workflow id")),
			mcp.WithNumber("retries", mcp.Description("Retries per step (default 2)")),
		), m.handleWorkflowRun)

	m.addTool("workflow.status", "Get workflow state. Read-only.", security.CatRead,
		mcp.NewTool("workflow.status",
			mcp.WithDescription("Return workflow state."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Workflow id")),
		), m.handleWorkflowStatus)

	m.addTool("workflow.cancel", "Cancel a workflow. DESTRUCTIVE.", security.CatDestruct,
		mcp.NewTool("workflow.cancel",
			mcp.WithDescription("Mark a workflow failed/cancelled."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Workflow id")),
		), m.handleWorkflowCancel)

	m.addTool("task.list", "List persisted tasks/workflows. Read-only.", security.CatRead,
		mcp.NewTool("task.list", mcp.WithDescription("List stored workflow/task ids.")), m.handleTaskList)

	m.addTool("task.get", "Get a task/workflow state. Read-only.", security.CatRead,
		mcp.NewTool("task.get",
			mcp.WithDescription("Return a task/workflow state."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Task id")),
		), m.handleTaskGet)

	m.addTool("task.retry", "Retry a failed workflow from the start. Side-effect.", security.CatWrite,
		mcp.NewTool("task.retry",
			mcp.WithDescription("Reset and re-run a failed workflow."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Task id")),
		), m.handleTaskRetry)

	// ---- Ops Tools (quality gate, audit, tracker) ----
	m.addTool("ops.qualitygate", "Run lint → vet → build → test quality gate. Read-only.", security.CatRead,
		mcp.NewTool("ops.qualitygate",
			mcp.WithDescription("Executes gofmt, go vet, go build, go test against a repo. Supports parallel mode."),
			mcp.WithString("repo_path", mcp.Description("Repository path (default: .)")),
			mcp.WithBoolean("parallel", mcp.Description("Run steps in parallel (default: false)")),
		), m.handleOpsQualityGate)

	m.addTool("tibrain.batch", "Batch multiple read-only MCP tool calls into a single request, executing them in parallel. Read-only.", security.CatRead,
		mcp.NewTool("tibrain.batch",
			mcp.WithDescription("Batch multiple read-only MCP tool calls into a single request for parallel execution, reducing latency."),
			mcp.WithString("operations_json", mcp.Required(), mcp.Description("JSON array of {tool, params} operations")),
		), m.handleBatch)

	m.addTool("subagent.context_status", "Check subagent context budget. Read-only.", security.CatRead,
		mcp.NewTool("subagent.context_status",
			mcp.WithDescription("Returns current context usage percentage, available tokens, and compaction recommendations for subagent sessions."),
			mcp.WithString("session_id", mcp.Description("Subagent session ID (optional)")),
		), m.handleContextStatus)

	m.addTool("ops.audit", "Scan repo for secrets and binary artifacts. Read-only.", security.CatRead,
		mcp.NewTool("ops.audit",
			mcp.WithDescription("Walks the repo tree, flags .env with API keys, .exe in root, credential files."),
			mcp.WithString("repo_path", mcp.Description("Repository path (default: .)")),
		), m.handleOpsAudit)

	m.addTool("ops.handoff", "Record a handoff entry. Side-effect.", security.CatWrite,
		mcp.NewTool("ops.handoff",
			mcp.WithDescription("Logs agent+action+details to handoff.json for session continuity."),
			mcp.WithString("agent", mcp.Required(), mcp.Description("Agent name")),
			mcp.WithString("action", mcp.Required(), mcp.Description("Action performed")),
		), m.handleOpsTrackerHandoff)

	m.addTool("ops.errors", "List recent error ledger entries. Read-only.", security.CatRead,
		mcp.NewTool("ops.errors",
			mcp.WithDescription("Returns recent deduplicated errors from the error ledger."),
			mcp.WithNumber("limit", mcp.Description("Max entries (default 10)")),
		), m.handleOpsTrackerErrors)

	m.addTool("ops.handoffs", "List recent handoff entries. Read-only.", security.CatRead,
		mcp.NewTool("ops.handoffs",
			mcp.WithDescription("Returns recent handoff log entries."),
			mcp.WithNumber("limit", mcp.Description("Max entries (default 10)")),
		), m.handleOpsRecentHandoffs)

	m.addTool("checkpoint.save", "Save session state to checkpoint for resume. Writes to disk.", security.CatWrite,
		mcp.NewTool("checkpoint.save",
			mcp.WithDescription("Writes checkpoint.md + flush learnings to memory. Resume via actor(context=\"state\")."),
			mcp.WithString("session_id", mcp.Required(), mcp.Description("Session to checkpoint")),
			mcp.WithString("progress_summary", mcp.Description("Progress to save")),
			mcp.WithString("learnings", mcp.Description("Key learnings to persist")),
		), m.handleCheckpointSave)

	m.addTool("subagent.flush", "Auto-flush subagent state at 60% context. Writes to disk.", security.CatWrite,
		mcp.NewTool("subagent.flush",
			mcp.WithDescription("Check context %, checkpoint if >=60%. Used by subagents for context management."),
			mcp.WithString("session_id", mcp.Required(), mcp.Description("Subagent session ID")),
			mcp.WithNumber("context_percent", mcp.Description("Current context usage %")),
			mcp.WithString("learnings", mcp.Description("Learnings to flush")),
			mcp.WithString("parent_actor_id", mcp.Description("Parent to signal after flush")),
		), m.handleSubagentFlush)

	m.addTool("memory.flush", "Quick learnings flush to global/MEMORY.md + One Store. Writes to disk.", security.CatWrite,
		mcp.NewTool("memory.flush",
			mcp.WithDescription("Append learnings to global/MEMORY.md and One Store (SQLite FTS5). Dedupes on existing entries. Auto-promotion eligible."),
			mcp.WithString("domain", mcp.Required(), mcp.Description("Knowledge domain key")),
			mcp.WithString("content", mcp.Required(), mcp.Description("Learning content to append")),
			mcp.WithNumber("confidence", mcp.Description("Confidence score 0.8-0.95")),
		), m.handleMemoryFlush)
}
