package android

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/ti/router/tibrain/internal/security"
)

// AndroidToolRegistrar provides functions to register Android MCP tools
type AndroidToolRegistrar struct {
	manager *AndroidManager
}

// NewAndroidToolRegistrar creates a new registrar
func NewAndroidToolRegistrar(am *AndroidManager) *AndroidToolRegistrar {
	return &AndroidToolRegistrar{manager: am}
}

// RegisterTools registers all Android MCP tools with the MCP manager
func (r *AndroidToolRegistrar) RegisterTools(addTool func(name, desc string, cat security.Category, tool mcp.Tool, handler server.ToolHandlerFunc)) {
	// ---- Android Device Info ----
	addTool("android.device_info", "Get comprehensive Android device information", security.CatRead,
		mcp.NewTool("android.device_info",
			mcp.WithDescription("Get comprehensive Android device information including model, Android version, battery, root status, Termux availability"),
		), r.handleAndroidDeviceInfo)

	// ---- ADB Shell Execution ----
	addTool("android.adb_shell", "Execute an ADB shell command on the connected Android device", security.CatWrite,
		mcp.NewTool("android.adb_shell",
			mcp.WithDescription("Execute an ADB shell command on the connected Android device"),
			mcp.WithString("command", mcp.Required(), mcp.Description("Command to execute (e.g., 'ls', 'pm list packages')")),
			mcp.WithArray("args", mcp.Description("Command arguments")),
			mcp.WithNumber("timeout_ms", mcp.Description("Timeout in ms (default 30000)")),
		), r.handleADBShell)

	// ---- Termux Command Execution ----
	addTool("android.termux_exec", "Execute a command in Termux on the Android device", security.CatWrite,
		mcp.NewTool("android.termux_exec",
			mcp.WithDescription("Execute a command in Termux on the Android device"),
			mcp.WithString("command", mcp.Required(), mcp.Description("Command to execute in Termux (e.g., 'python script.py')")),
			mcp.WithNumber("timeout_ms", mcp.Description("Timeout in ms (default 30000)")),
		), r.handleTermuxExec)

	// ---- File Operations ----
	addTool("android.pull_file", "Pull a file from Android device to local machine", security.CatRead,
		mcp.NewTool("android.pull_file",
			mcp.WithDescription("Pull a file from Android device to local machine"),
			mcp.WithString("remote_path", mcp.Required(), mcp.Description("Remote path on Android device")),
			mcp.WithString("local_path", mcp.Required(), mcp.Description("Local destination path")),
		), r.handlePullFile)

	addTool("android.push_file", "Push a file from local machine to Android device", security.CatWrite,
		mcp.NewTool("android.push_file",
			mcp.WithDescription("Push a file from local machine to Android device"),
			mcp.WithString("local_path", mcp.Required(), mcp.Description("Local source path")),
			mcp.WithString("remote_path", mcp.Required(), mcp.Description("Remote destination path on Android device")),
		), r.handlePushFile)

	// ---- Screen Capture ----
	addTool("android.screenshot", "Take a screenshot of the Android device", security.CatRead,
		mcp.NewTool("android.screenshot",
			mcp.WithDescription("Take a screenshot of the Android device and save locally"),
			mcp.WithString("local_path", mcp.Required(), mcp.Description("Local path to save screenshot")),
		), r.handleScreenshot)

	// ---- Package Management ----
	addTool("android.list_packages", "List installed packages on Android device", security.CatRead,
		mcp.NewTool("android.list_packages",
			mcp.WithDescription("List installed packages on Android device"),
			mcp.WithString("filter", mcp.Description("Optional package name filter")),
		), r.handleListPackages)

	// ---- Intent Execution ----
	addTool("android.send_intent", "Send an Android intent", security.CatWrite,
		mcp.NewTool("android.send_intent",
			mcp.WithDescription("Send an Android intent to trigger an action"),
			mcp.WithString("action", mcp.Required(), mcp.Description("Intent action (e.g., android.intent.action.VIEW)")),
			mcp.WithObject("extras", mcp.Description("Intent extras as key-value pairs")),
		), r.handleSendIntent)

	// ---- Logcat ----
	addTool("android.logcat", "Retrieve logcat output from Android device", security.CatRead,
		mcp.NewTool("android.logcat",
			mcp.WithDescription("Retrieve logcat output from Android device"),
			mcp.WithString("filter", mcp.Description("Log filter (e.g., '*:E' for errors only)")),
			mcp.WithNumber("lines", mcp.Description("Number of lines to retrieve (default 100)")),
		), r.handleLogcat)

	// ---- Device Discovery ----
	addTool("android.list_devices", "List all connected ADB devices", security.CatRead,
		mcp.NewTool("android.list_devices",
			mcp.WithDescription("List all connected ADB devices"),
		), r.handleListDevices)
}

// ---- Tool Handlers ----

func (r *AndroidToolRegistrar) handleAndroidDeviceInfo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	info, err := r.manager.GetDeviceInfo(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (r *AndroidToolRegistrar) handleADBShell(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	command := req.GetString("command", "")
	args := req.GetStringSlice("args", nil)
	_ = req.GetInt("timeout_ms", 30000)

	if command == "" {
		return mcp.NewToolResultError("command is required"), nil
	}

	output, err := r.manager.RunADBCommand(ctx, command, args...)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(output), nil
}

func (r *AndroidToolRegistrar) handleTermuxExec(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	command := req.GetString("command", "")
	_ = req.GetInt("timeout_ms", 30000)

	if command == "" {
		return mcp.NewToolResultError("command is required"), nil
	}

	output, err := r.manager.RunTermuxCommand(ctx, command)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(output), nil
}

func (r *AndroidToolRegistrar) handlePullFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	remotePath := req.GetString("remote_path", "")
	localPath := req.GetString("local_path", "")

	if remotePath == "" || localPath == "" {
		return mcp.NewToolResultError("remote_path and local_path are required"), nil
	}

	err := r.manager.PullFile(ctx, remotePath, localPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("File pulled from %s to %s", remotePath, localPath)), nil
}

func (r *AndroidToolRegistrar) handlePushFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	localPath := req.GetString("local_path", "")
	remotePath := req.GetString("remote_path", "")

	if localPath == "" || remotePath == "" {
		return mcp.NewToolResultError("local_path and remote_path are required"), nil
	}

	err := r.manager.PushFile(ctx, localPath, remotePath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("File pushed from %s to %s", localPath, remotePath)), nil
}

func (r *AndroidToolRegistrar) handleScreenshot(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	localPath := req.GetString("local_path", "")
	if localPath == "" {
		return mcp.NewToolResultError("local_path is required"), nil
	}

	err := r.manager.TakeScreenshot(ctx, localPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Screenshot saved to %s", localPath)), nil
}

func (r *AndroidToolRegistrar) handleListPackages(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filter := req.GetString("filter", "")
	packages, err := r.manager.ListPackages(ctx, filter)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	data, _ := json.MarshalIndent(packages, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (r *AndroidToolRegistrar) handleSendIntent(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := req.GetString("action", "")
	if action == "" {
		return mcp.NewToolResultError("action is required"), nil
	}

	// Get extras from arguments
	args := req.GetArguments()
	extrasMap := make(map[string]string)
	if extrasVal, ok := args["extras"]; ok {
		if extrasMapVal, ok := extrasVal.(map[string]interface{}); ok {
			for k, v := range extrasMapVal {
				if s, ok := v.(string); ok {
					extrasMap[k] = s
				}
			}
		}
	}

	err := r.manager.ExecuteIntent(ctx, action, extrasMap)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Intent %s sent successfully", action)), nil
}

func (r *AndroidToolRegistrar) handleLogcat(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filter := req.GetString("filter", "")
	lines := req.GetInt("lines", 100)

	output, err := r.manager.GetLogcat(ctx, filter, lines)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(output), nil
}

func (r *AndroidToolRegistrar) handleListDevices(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	devices, err := ListDevices()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	data, _ := json.MarshalIndent(devices, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}
