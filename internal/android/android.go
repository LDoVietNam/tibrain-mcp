package android

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// AndroidManager manages communication with Android devices via ADB/Termux
type AndroidManager struct {
	adbPath      string
	termuxSocket string
	deviceSerial string
	timeout      time.Duration
}

// AndroidConfig holds configuration for Android connection
type AndroidConfig struct {
	ADBPath      string
	TermuxSocket string
	DeviceSerial string
	Timeout      time.Duration
}

// NewAndroidManager creates a new Android manager
func NewAndroidManager(cfg AndroidConfig) *AndroidManager {
	adbPath := cfg.ADBPath
	if adbPath == "" {
		adbPath = "adb"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &AndroidManager{
		adbPath:      adbPath,
		termuxSocket: cfg.TermuxSocket,
		deviceSerial: cfg.DeviceSerial,
		timeout:      timeout,
	}
}

// DeviceInfo represents Android device information
type DeviceInfo struct {
	Serial       string   `json:"serial"`
	Model        string   `json:"model"`
	Manufacturer string   `json:"manufacturer"`
	AndroidVer   string   `json:"android_version"`
	SDKVersion   string   `json:"sdk_version"`
	CPU          string   `json:"cpu_abi"`
	ScreenRes    string   `json:"screen_resolution"`
	BatteryLevel int      `json:"battery_level"`
	BatteryState string   `json:"battery_state"`
	IsRooted     bool     `json:"is_rooted"`
	TermuxAvail  bool     `json:"termux_available"`
	IPAddresses  []string `json:"ip_addresses"`
}

// GetDeviceInfo retrieves comprehensive device information
func (m *AndroidManager) GetDeviceInfo(ctx context.Context) (*DeviceInfo, error) {
	// Get basic device properties
	props := []string{
		"ro.product.model",
		"ro.product.manufacturer",
		"ro.build.version.release",
		"ro.build.version.sdk",
		"ro.product.cpu.abi",
		"ro.build.display.id",
	}

	deviceInfo := &DeviceInfo{
		Serial: m.deviceSerial,
	}

	// Get properties via ADB
	for _, prop := range props {
		out, err := m.runADB(ctx, "shell", "getprop", prop)
		if err != nil {
			continue
		}
		val := strings.TrimSpace(string(out))
		switch prop {
		case "ro.product.model":
			deviceInfo.Model = val
		case "ro.product.manufacturer":
			deviceInfo.Manufacturer = val
		case "ro.build.version.release":
			deviceInfo.AndroidVer = val
		case "ro.build.version.sdk":
			deviceInfo.SDKVersion = val
		case "ro.product.cpu.abi":
			deviceInfo.CPU = val
		}
	}

	// Get screen resolution
	if out, err := m.runADB(ctx, "shell", "wm", "size"); err == nil {
		deviceInfo.ScreenRes = strings.TrimSpace(strings.TrimPrefix(string(out), "Physical size:"))
	}

	// Get battery info
	if out, err := m.runADB(ctx, "shell", "dumpsys", "battery"); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "level:") {
				fmt.Sscanf(line, "level: %d", &deviceInfo.BatteryLevel)
			}
			if strings.HasPrefix(line, "status:") {
				statuses := map[string]string{
					"2": "charging",
					"3": "discharging",
					"4": "full",
					"5": "not_charging",
				}
				if s := strings.TrimSpace(strings.TrimPrefix(line, "status:")); s != "" {
					deviceInfo.BatteryState = statuses[s]
				}
			}
		}
	}

	// Check root access
	if out, err := m.runADB(ctx, "shell", "which", "su"); err == nil && strings.TrimSpace(string(out)) != "" {
		deviceInfo.IsRooted = true
	}

	// Check Termux availability
	if out, err := m.runADB(ctx, "shell", "pm", "list", "packages", "com.termux"); err == nil {
		deviceInfo.TermuxAvail = strings.Contains(string(out), "com.termux")
	}

	// Get IP addresses
	if _, err := m.runADB(ctx, "shell", "ip", "addr", "show"); err == nil {
		// Parse IP addresses from output
		// Simplified - just note we got them
		deviceInfo.IPAddresses = []string{"available"}
	}

	return deviceInfo, nil
}

// RunADBCommand executes an ADB shell command
func (m *AndroidManager) RunADBCommand(ctx context.Context, command string, args ...string) (string, error) {
	allArgs := append([]string{"shell"}, append([]string{command}, args...)...)
	return m.runADB(ctx, allArgs...)
}

// RunTermuxCommand executes a command in Termux
func (m *AndroidManager) RunTermuxCommand(ctx context.Context, command string) (string, error) {
	// Check if Termux is available
	if _, err := m.runADB(ctx, "shell", "pm", "list", "packages", "com.termux"); err != nil {
		return "", fmt.Errorf("Termux not installed on device")
	}

	// Execute command in Termux
	return m.runADB(ctx, "shell", "am", "start", "-n", "com.termux/.app.TermuxActivity", "-e", "command", command)
}

// PullFile pulls a file from device to local
func (m *AndroidManager) PullFile(ctx context.Context, remotePath, localPath string) error {
	args := []string{"pull", remotePath, localPath}
	if m.deviceSerial != "" {
		args = append([]string{"-s", m.deviceSerial}, args...)
	}
	cmd := exec.CommandContext(ctx, m.adbPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb pull failed: %w, output: %s", err, string(output))
	}
	return nil
}

// PushFile pushes a file from local to device
func (m *AndroidManager) PushFile(ctx context.Context, localPath, remotePath string) error {
	args := []string{"push", localPath, remotePath}
	if m.deviceSerial != "" {
		args = append([]string{"-s", m.deviceSerial}, args...)
	}
	cmd := exec.CommandContext(ctx, m.adbPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb push failed: %w, output: %s", err, string(output))
	}
	return nil
}

// TakeScreenshot takes a screenshot and saves to local path
func (m *AndroidManager) TakeScreenshot(ctx context.Context, localPath string) error {
	remotePath := "/sdcard/screenshot_" + time.Now().Format("20060102_150405") + ".png"

	// Take screenshot on device
	if _, err := m.runADB(ctx, "shell", "screencap", "-p", remotePath); err != nil {
		return fmt.Errorf("failed to take screenshot: %w", err)
	}

	// Pull to local
	if err := m.PullFile(ctx, remotePath, localPath); err != nil {
		return err
	}

	// Clean up remote
	m.runADB(ctx, "shell", "rm", remotePath)
	return nil
}

// ListPackages lists installed packages
func (m *AndroidManager) ListPackages(ctx context.Context, filter string) ([]string, error) {
	args := []string{"shell", "pm", "list", "packages"}
	if filter != "" {
		args = append(args, filter)
	}
	out, err := m.runADB(ctx, args...)
	if err != nil {
		return nil, err
	}

	var packages []string
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package:") {
			packages = append(packages, strings.TrimPrefix(line, "package:"))
		}
	}
	return packages, nil
}

// ExecuteIntent sends an Android intent
func (m *AndroidManager) ExecuteIntent(ctx context.Context, action string, extras map[string]string) error {
	args := []string{"shell", "am", "start", "-a", action}
	for k, v := range extras {
		args = append(args, "-e", k, v)
	}
	_, err := m.runADB(ctx, args...)
	return err
}

// GetLogcat retrieves logcat output
func (m *AndroidManager) GetLogcat(ctx context.Context, filter string, lines int) (string, error) {
	args := []string{"logcat", "-d"}
	if lines > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", lines))
	}
	if filter != "" {
		args = append(args, filter)
	}
	return m.runADB(ctx, args...)
}

// runADB executes an ADB command with proper context and timeout
func (m *AndroidManager) runADB(ctx context.Context, args ...string) (string, error) {
	cmdArgs := args
	if m.deviceSerial != "" {
		cmdArgs = append([]string{"-s", m.deviceSerial}, cmdArgs...)
	}

	cmd := exec.CommandContext(ctx, m.adbPath, cmdArgs...)

	// Set timeout
	if m.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.timeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, m.adbPath, cmdArgs...)
	}

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		// Check if it's a timeout
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("adb command timed out after %v", m.timeout)
		}
		return "", fmt.Errorf("adb command failed: %w, output: %s", err, outputStr)
	}

	return outputStr, nil
}

// ListDevices lists all connected ADB devices
func ListDevices() ([]DeviceInfo, error) {
	cmd := exec.Command("adb", "devices", "-l")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var devices []DeviceInfo
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "device" {
			devices = append(devices, DeviceInfo{
				Serial: fields[0],
			})
		}
	}
	return devices, nil
}

// CheckADB checks if ADB is available and working
func CheckADB() error {
	cmd := exec.Command("adb", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb not available: %w", err)
	}
	version := strings.TrimSpace(string(output))
	fmt.Printf("ADB version: %s\n", version)
	return nil
}

// MarshalJSON implements custom JSON marshaling for DeviceInfo
func (d *DeviceInfo) MarshalJSON() ([]byte, error) {
	type Alias DeviceInfo
	return json.Marshal(&struct {
		*Alias
		Timestamp int64 `json:"timestamp"`
	}{
		Alias:     (*Alias)(d),
		Timestamp: time.Now().Unix(),
	})
}
