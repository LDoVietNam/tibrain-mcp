package memory

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDeviceRegistry_Init(t *testing.T) {
	tempDir := t.TempDir()

	registry, err := NewDeviceRegistry(tempDir, nil)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	current := registry.GetCurrentDevice()
	if current == nil {
		t.Fatal("Expected current device to be set")
	}

	if current.DeviceID == "" {
		t.Error("Expected device ID to be set")
	}

	if current.DeviceName == "" {
		t.Error("Expected device name to be set")
	}

	if !current.Trusted {
		t.Error("Expected first device to be trusted")
	}

	if !current.SyncEnabled {
		t.Error("Expected sync to be enabled by default")
	}
}

func TestDeviceRegistry_RegisterDevice(t *testing.T) {
	tempDir := t.TempDir()

	registry, err := NewDeviceRegistry(tempDir, nil)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	device, err := registry.RegisterDevice("test-device", "mobile")
	if err != nil {
		t.Fatalf("Failed to register device: %v", err)
	}

	if device.DeviceID == "" {
		t.Error("Expected device ID to be set")
	}

	if device.DeviceName != "test-device" {
		t.Errorf("Expected name 'test-device', got '%s'", device.DeviceName)
	}

	if device.DeviceType != "mobile" {
		t.Errorf("Expected type 'mobile', got '%s'", device.DeviceType)
	}

	if device.Trusted {
		t.Error("Expected new device to not be trusted by default")
	}

	// Verify it's in the list
	devices := registry.ListDevices()
	found := false
	for _, d := range devices {
		if d.DeviceID == device.DeviceID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Registered device not found in list")
	}
}

func TestDeviceRegistry_PersistAndLoad(t *testing.T) {
	tempDir := t.TempDir()

	// Create first registry and register a device
	registry1, err := NewDeviceRegistry(tempDir, nil)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	device, err := registry1.RegisterDevice("persist-test", "desktop")
	if err != nil {
		t.Fatalf("Failed to register device: %v", err)
	}
	deviceID := device.DeviceID

	// Create new registry from same path
	registry2, err := NewDeviceRegistry(tempDir, nil)
	if err != nil {
		t.Fatalf("Failed to load registry: %v", err)
	}

	// Check current device persisted
	current := registry2.GetCurrentDevice()
	if current == nil {
		t.Fatal("Expected current device to be loaded")
	}

	// Check registered device persisted
	loaded, ok := registry2.GetDevice(deviceID)
	if !ok {
		t.Fatalf("Expected device %s to be loaded", deviceID)
	}

	if loaded.DeviceName != "persist-test" {
		t.Errorf("Expected name 'persist-test', got '%s'", loaded.DeviceName)
	}
}

func TestDeviceRegistry_TrustDevice(t *testing.T) {
	tempDir := t.TempDir()

	registry, err := NewDeviceRegistry(tempDir, nil)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	device, err := registry.RegisterDevice("untrusted-device", "mobile")
	if err != nil {
		t.Fatalf("Failed to register device: %v", err)
	}

	if device.Trusted {
		t.Error("Expected new device to be untrusted")
	}

	// Trust the device
	err = registry.TrustDevice(device.DeviceID)
	if err != nil {
		t.Fatalf("Failed to trust device: %v", err)
	}

	// Verify trusted
	trusted, ok := registry.GetDevice(device.DeviceID)
	if !ok {
		t.Fatal("Device not found after trust")
	}
	if !trusted.Trusted {
		t.Error("Expected device to be trusted")
	}
}

func TestDeviceRegistry_EnableSync(t *testing.T) {
	tempDir := t.TempDir()

	registry, err := NewDeviceRegistry(tempDir, nil)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	device, err := registry.RegisterDevice("sync-test", "desktop")
	if err != nil {
		t.Fatalf("Failed to register device: %v", err)
	}

	// Disable sync
	err = registry.EnableSync(device.DeviceID, false)
	if err != nil {
		t.Fatalf("Failed to disable sync: %v", err)
	}

	disabled, ok := registry.GetDevice(device.DeviceID)
	if !ok {
		t.Fatal("Device not found")
	}
	if disabled.SyncEnabled {
		t.Error("Expected sync to be disabled")
	}

	// Re-enable
	err = registry.EnableSync(device.DeviceID, true)
	if err != nil {
		t.Fatalf("Failed to enable sync: %v", err)
	}

	enabled, ok := registry.GetDevice(device.DeviceID)
	if !ok {
		t.Fatal("Device not found")
	}
	if !enabled.SyncEnabled {
		t.Error("Expected sync to be enabled")
	}
}

func TestDeviceRegistry_UpdateLastSync(t *testing.T) {
	tempDir := t.TempDir()

	registry, err := NewDeviceRegistry(tempDir, nil)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	current := registry.GetCurrentDevice()
	if current.LastSyncAt.IsZero() {
		// Expected initially
	}

	// Update last sync
	before := time.Now()
	err = registry.UpdateLastSync(current.DeviceID)
	if err != nil {
		t.Fatalf("Failed to update last sync: %v", err)
	}
	after := time.Now()

	updated, ok := registry.GetDevice(current.DeviceID)
	if !ok {
		t.Fatal("Device not found")
	}
	if updated.LastSyncAt.Before(before) || updated.LastSyncAt.After(after) {
		t.Errorf("Last sync time not updated correctly: %v", updated.LastSyncAt)
	}
}

func TestDeviceRegistry_RemoveDevice(t *testing.T) {
	tempDir := t.TempDir()

	registry, err := NewDeviceRegistry(tempDir, nil)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	device, err := registry.RegisterDevice("to-remove", "mobile")
	if err != nil {
		t.Fatalf("Failed to register device: %v", err)
	}

	// Try to remove current device (should fail)
	current := registry.GetCurrentDevice()
	err = registry.RemoveDevice(current.DeviceID)
	if err == nil {
		t.Error("Expected error when removing current device")
	}

	// Remove the other device
	err = registry.RemoveDevice(device.DeviceID)
	if err != nil {
		t.Fatalf("Failed to remove device: %v", err)
	}

	// Verify removed
	_, ok := registry.GetDevice(device.DeviceID)
	if ok {
		t.Error("Expected device to be removed")
	}
}

func TestDeviceRegistry_ListDevices(t *testing.T) {
	tempDir := t.TempDir()

	registry, err := NewDeviceRegistry(tempDir, nil)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Register a few devices
	_, err = registry.RegisterDevice("device-1", "desktop")
	if err != nil {
		t.Fatalf("Failed to register device: %v", err)
	}
	_, err = registry.RegisterDevice("device-2", "mobile")
	if err != nil {
		t.Fatalf("Failed to register device: %v", err)
	}

	devices := registry.ListDevices()
	// Should have current + 2 registered = 3
	if len(devices) != 3 {
		t.Errorf("Expected 3 devices, got %d", len(devices))
	}
}

func TestSyncCoordinator_SyncAllDevices(t *testing.T) {
	tempDir := t.TempDir()

	// Create a mock setup
	engine := NewPromotionEngine(filepath.Join(tempDir, "memory"), nil)
	registry, err := NewDeviceRegistry(filepath.Join(tempDir, "devices"), nil)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Need a real repo for syncer
	repoPath := filepath.Join(tempDir, "sync_repo")
	syncer := NewSyncer(engine, DefaultSyncConfig(repoPath), nil)

	coordinator := NewSyncCoordinator(engine, registry, syncer, nil)

	// Register a trusted device
	device, err := registry.RegisterDevice("test-device", "desktop")
	if err != nil {
		t.Fatalf("Failed to register device: %v", err)
	}
	registry.TrustDevice(device.DeviceID)
	registry.EnableSync(device.DeviceID, true)

	// Sync all devices (will fail without actual git remote, but should not panic)
	_, err = coordinator.SyncAllDevices()
	if err != nil {
		// Expected to fail without git remote, but should return results map
	}

	// Check status was tracked
	status := coordinator.GetSyncStatus()
	if len(status) == 0 {
		t.Error("Expected sync status to be tracked")
	}
}

func TestConflictResolver_ResolveMultiDeviceConflict(t *testing.T) {
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "sync_repo")

	engine := NewPromotionEngine(filepath.Join(tempDir, "memory"), nil)
	syncer := NewSyncer(engine, DefaultSyncConfig(repoPath), nil)
	resolver := NewConflictResolver(syncer, nil)

	now := time.Now()

	// Create multiple versions of same entry
	versions := map[string]*TieredMemoryEntry{
		"device-1": {
			Name:       "conflict-entry",
			Domain:     "test",
			Content:    "Version from device 1",
			Confidence: 0.9,
			Verified:   true,
			Tier:       TierCore,
			Tags:       []string{"device1", "shared"},
			Version:    "1.0",
			UpdatedAt:  now,
		},
		"device-2": {
			Name:       "conflict-entry",
			Domain:     "test",
			Content:    "Version from device 2",
			Confidence: 0.7, // Lower confidence
			Verified:   true,
			Tier:       TierCore,
			Tags:       []string{"device2", "shared"},
			Version:    "1.0",
			UpdatedAt:  now.Add(-time.Hour),
		},
		"device-3": {
			Name:       "conflict-entry",
			Domain:     "test",
			Content:    "Version from device 3",
			Confidence: 0.8,
			Verified:   true,
			Tier:       TierCore,
			Tags:       []string{"device3", "shared"},
			Version:    "1.0",
			UpdatedAt:  now.Add(-2 * time.Hour),
		},
	}

	merged, err := resolver.ResolveMultiDeviceConflict("conflict-entry", versions)
	if err != nil {
		t.Fatalf("ResolveMultiDeviceConflict failed: %v", err)
	}

	// Winner should be device-1 (highest confidence 0.9)
	if merged.Confidence != 0.9 {
		t.Errorf("Expected winner confidence 0.9, got %f", merged.Confidence)
	}

	// Should have merged tags from all devices
	expectedTags := map[string]bool{"device1": true, "device2": true, "device3": true, "shared": true}
	if len(merged.Tags) != len(expectedTags) {
		t.Errorf("Expected %d merged tags, got %d: %v", len(expectedTags), len(merged.Tags), merged.Tags)
	}
	for _, tag := range merged.Tags {
		if !expectedTags[tag] {
			t.Errorf("Unexpected tag in merged result: %s", tag)
		}
	}

	// Version should be updated
	if merged.Version == "1.0" {
		t.Error("Expected version to be updated after merge")
	}
}

func TestConflictResolver_SingleVersion(t *testing.T) {
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "sync_repo")

	engine := NewPromotionEngine(filepath.Join(tempDir, "memory"), nil)
	syncer := NewSyncer(engine, DefaultSyncConfig(repoPath), nil)
	resolver := NewConflictResolver(syncer, nil)

	versions := map[string]*TieredMemoryEntry{
		"device-1": {
			Name:       "single-entry",
			Domain:     "test",
			Content:    "Single version",
			Confidence: 0.8,
			Verified:   true,
			Tier:       TierCore,
			Tags:       []string{"tag1"},
			Version:    "1.0",
		},
	}

	merged, err := resolver.ResolveMultiDeviceConflict("single-entry", versions)
	if err != nil {
		t.Fatalf("ResolveMultiDeviceConflict failed: %v", err)
	}

	if merged.Name != "single-entry" {
		t.Errorf("Expected single-entry, got %s", merged.Name)
	}
}

func TestGenerateDeviceID(t *testing.T) {
	id1 := generateDeviceID()
	id2 := generateDeviceID()

	if id1 == "" || id2 == "" {
		t.Error("Generated IDs should not be empty")
	}

	if id1 == id2 {
		t.Error("Generated IDs should be unique")
	}

	if len(id1) < 10 {
		t.Errorf("ID too short: %s", id1)
	}
}
