package memory

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// DeviceInfo represents a registered device/client
type DeviceInfo struct {
	DeviceID     string    `yaml:"device_id"`
	DeviceName   string    `yaml:"device_name"`
	DeviceType   string    `yaml:"device_type"` // "desktop", "mobile", "server", "cli"
	PublicKey    string    `yaml:"public_key,omitempty"`
	RegisteredAt time.Time `yaml:"registered_at"`
	LastSyncAt   time.Time `yaml:"last_sync_at"`
	SyncEnabled  bool      `yaml:"sync_enabled"`
	Trusted      bool      `yaml:"trusted"`
}

// DeviceRegistry manages device registration and identity
type DeviceRegistry struct {
	mu        sync.RWMutex
	devices   map[string]*DeviceInfo
	currentID string
	storePath string
	metrics   *MetricsCollector
}

// NewDeviceRegistry creates a new device registry
func NewDeviceRegistry(storePath string, metrics *MetricsCollector) (*DeviceRegistry, error) {
	registry := &DeviceRegistry{
		devices:   make(map[string]*DeviceInfo),
		storePath: filepath.Join(storePath, "devices.yaml"),
		metrics:   metrics,
	}

	// Load existing devices
	if err := registry.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// Generate or load current device ID
	if err := registry.initCurrentDevice(); err != nil {
		return nil, err
	}

	return registry, nil
}

// initCurrentDevice initializes or loads the current device identity
func (dr *DeviceRegistry) initCurrentDevice() error {
	// Check if we have a current device ID stored
	idPath := filepath.Join(filepath.Dir(dr.storePath), "current_device_id")
	if data, err := os.ReadFile(idPath); err == nil {
		dr.currentID = strings.TrimSpace(string(data))
		if _, exists := dr.devices[dr.currentID]; exists {
			if dr.metrics != nil {
				dr.metrics.UpdateDeviceMetrics(len(dr.devices), 1)
			}
			return nil
		}
	}

	// Generate new device ID
	dr.currentID = generateDeviceID()
	device := &DeviceInfo{
		DeviceID:     dr.currentID,
		DeviceName:   getHostname(),
		DeviceType:   "cli",
		RegisteredAt: time.Now(),
		LastSyncAt:   time.Time{},
		SyncEnabled:  true,
		Trusted:      true, // First device is trusted by default
	}

	dr.devices[dr.currentID] = device
	if dr.metrics != nil {
		dr.metrics.UpdateDeviceMetrics(len(dr.devices), 1)
	}
	return dr.save()
}

// RegisterDevice registers a new device (for multi-device setup)
func (dr *DeviceRegistry) RegisterDevice(name, deviceType string) (*DeviceInfo, error) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	deviceID := generateDeviceID()
	device := &DeviceInfo{
		DeviceID:     deviceID,
		DeviceName:   name,
		DeviceType:   deviceType,
		RegisteredAt: time.Now(),
		LastSyncAt:   time.Time{},
		SyncEnabled:  true,
		Trusted:      false, // New devices need approval
	}

	dr.devices[deviceID] = device
	if dr.metrics != nil {
		dr.metrics.UpdateDeviceMetrics(len(dr.devices), 0)
	}
	return device, dr.save()
}

// GetCurrentDevice returns the current device info
func (dr *DeviceRegistry) GetCurrentDevice() *DeviceInfo {
	dr.mu.RLock()
	defer dr.mu.RUnlock()
	return dr.devices[dr.currentID]
}

// GetDevice returns a device by ID
func (dr *DeviceRegistry) GetDevice(deviceID string) (*DeviceInfo, bool) {
	dr.mu.RLock()
	defer dr.mu.RUnlock()
	device, ok := dr.devices[deviceID]
	return device, ok
}

// ListDevices returns all registered devices
func (dr *DeviceRegistry) ListDevices() []*DeviceInfo {
	dr.mu.RLock()
	defer dr.mu.RUnlock()

	devices := make([]*DeviceInfo, 0, len(dr.devices))
	for _, d := range dr.devices {
		devices = append(devices, d)
	}
	return devices
}

// TrustDevice marks a device as trusted
func (dr *DeviceRegistry) TrustDevice(deviceID string) error {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	device, ok := dr.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	device.Trusted = true
	return dr.save()
}

// EnableSync enables/disables sync for a device
func (dr *DeviceRegistry) EnableSync(deviceID string, enabled bool) error {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	device, ok := dr.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	device.SyncEnabled = enabled
	return dr.save()
}

// UpdateLastSync updates the last sync timestamp for a device
func (dr *DeviceRegistry) UpdateLastSync(deviceID string) error {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	device, ok := dr.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	device.LastSyncAt = time.Now()
	return dr.save()
}

// RemoveDevice removes a device from registry
func (dr *DeviceRegistry) RemoveDevice(deviceID string) error {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if deviceID == dr.currentID {
		return fmt.Errorf("cannot remove current device")
	}

	delete(dr.devices, deviceID)
	if dr.metrics != nil {
		dr.metrics.UpdateDeviceMetrics(len(dr.devices), 0)
	}
	return dr.save()
}

// load loads devices from disk
func (dr *DeviceRegistry) load() error {
	data, err := os.ReadFile(dr.storePath)
	if err != nil {
		return err
	}

	var loaded struct {
		Devices   map[string]*DeviceInfo `yaml:"devices"`
		CurrentID string                 `yaml:"current_id"`
	}

	if err := yaml.Unmarshal(data, &loaded); err != nil {
		return err
	}

	dr.devices = loaded.Devices
	dr.currentID = loaded.CurrentID
	return nil
}

// save saves devices to disk (caller must hold dr.mu.Lock())
func (dr *DeviceRegistry) save() error {
	data := struct {
		Devices   map[string]*DeviceInfo `yaml:"devices"`
		CurrentID string                 `yaml:"current_id"`
	}{
		Devices:   dr.devices,
		CurrentID: dr.currentID,
	}

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dr.storePath), 0755); err != nil {
		return err
	}

	return os.WriteFile(dr.storePath, yamlData, 0644)
}

// generateDeviceID generates a cryptographically secure device ID
func generateDeviceID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("dev-%d", time.Now().UnixNano())
	}
	return "dev-" + hex.EncodeToString(bytes)
}

// getHostname returns the hostname
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown-host"
	}
	return hostname
}

// SyncCoordinator coordinates sync across multiple devices
type SyncCoordinator struct {
	engine     *PromotionEngine
	registry   *DeviceRegistry
	syncer     *Syncer
	mu         sync.Mutex
	lastSync   map[string]time.Time // deviceID -> last sync time
	syncStatus map[string]SyncStatus
	metrics    *MetricsCollector
}

// SyncStatus represents the sync status of a device
type SyncStatus string

const (
	SyncStatusIdle      SyncStatus = "idle"
	SyncStatusSyncing   SyncStatus = "syncing"
	SyncStatusConflict  SyncStatus = "conflict"
	SyncStatusError     SyncStatus = "error"
	SyncStatusOutOfSync SyncStatus = "out_of_sync"
)

// NewSyncCoordinator creates a new sync coordinator
func NewSyncCoordinator(engine *PromotionEngine, registry *DeviceRegistry, syncer *Syncer, metrics *MetricsCollector) *SyncCoordinator {
	return &SyncCoordinator{
		engine:     engine,
		registry:   registry,
		syncer:     syncer,
		lastSync:   make(map[string]time.Time),
		syncStatus: make(map[string]SyncStatus),
		metrics:    metrics,
	}
}

// SyncAllDevices performs sync for all trusted, enabled devices
func (sc *SyncCoordinator) SyncAllDevices() (map[string]*SyncResult, error) {
	start := time.Now()
	devices := sc.registry.ListDevices()
	results := make(map[string]*SyncResult)

	for _, device := range devices {
		if !device.Trusted || !device.SyncEnabled {
			continue
		}

		sc.setStatus(device.DeviceID, SyncStatusSyncing)

		// Update device's last sync time
		sc.registry.UpdateLastSync(device.DeviceID)

		result, err := sc.syncForDevice(device)
		if err != nil {
			sc.setStatus(device.DeviceID, SyncStatusError)
			result = &SyncResult{
				Errors: []error{err},
			}
		} else if len(result.Conflicts) > 0 {
			sc.setStatus(device.DeviceID, SyncStatusConflict)
		} else {
			sc.setStatus(device.DeviceID, SyncStatusIdle)
		}

		results[device.DeviceID] = result
		sc.lastSync[device.DeviceID] = time.Now()
	}

	if sc.metrics != nil {
		sc.metrics.RecordSync("device_coordinator", "success", time.Since(start))
	}

	return results, nil
}

// syncForDevice performs sync for a specific device
// In a real implementation, this would coordinate with the device's repo
func (sc *SyncCoordinator) syncForDevice(device *DeviceInfo) (*SyncResult, error) {
	// For now, delegate to the main syncer
	// In a multi-device setup, each device would have its own repo remote
	return sc.syncer.FullSync()
}

// GetSyncStatus returns sync status for all devices
func (sc *SyncCoordinator) GetSyncStatus() map[string]SyncStatus {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	status := make(map[string]SyncStatus)
	for k, v := range sc.syncStatus {
		status[k] = v
	}
	return status
}

// GetLastSync returns last sync time for all devices
func (sc *SyncCoordinator) GetLastSync() map[string]time.Time {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	lastSync := make(map[string]time.Time)
	for k, v := range sc.lastSync {
		lastSync[k] = v
	}
	return lastSync
}

// setStatus updates sync status for a device
func (sc *SyncCoordinator) setStatus(deviceID string, status SyncStatus) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.syncStatus[deviceID] = status
}

// StartBackgroundSync starts periodic sync for all devices
func (sc *SyncCoordinator) StartBackgroundSync(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			if _, err := sc.SyncAllDevices(); err != nil {
				// Log error but continue
				continue
			}
		}
	}()
}

// ConflictResolver handles multi-device conflict resolution
type ConflictResolver struct {
	syncer  *Syncer
	metrics *MetricsCollector
}

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver(syncer *Syncer, metrics *MetricsCollector) *ConflictResolver {
	return &ConflictResolver{syncer: syncer, metrics: metrics}
}

// ResolveMultiDeviceConflict resolves conflicts between multiple device versions
func (cr *ConflictResolver) ResolveMultiDeviceConflict(entryName string, versions map[string]*TieredMemoryEntry) (*TieredMemoryEntry, error) {
	if len(versions) <= 1 {
		// No conflict
		for _, v := range versions {
			return v, nil
		}
		return nil, fmt.Errorf("no versions provided")
	}

	// Strategy 1: Highest confidence wins
	var winner *TieredMemoryEntry
	var winnerDevice string
	maxConf := -1.0

	for deviceID, version := range versions {
		if version.Confidence > maxConf {
			maxConf = version.Confidence
			winner = version
			winnerDevice = deviceID
		}
	}

	// Strategy 2: Merge unique content from all versions
	merged := cr.mergeAllVersions(winner, versions)

	// Strategy 3: Update version and timestamp
	merged.Version = fmt.Sprintf("%s.merged-%d", merged.Version, time.Now().Unix())
	merged.UpdatedAt = time.Now()
	merged.NeedsSave = true

	// Log the resolution
	conflict := SyncConflict{
		EntryName:     entryName,
		LocalVersion:  winner.Version,
		RemoteVersion: "multi-device-merge",
		LocalConf:     winner.Confidence,
		RemoteConf:    0, // N/A for multi-device
		Resolution:    fmt.Sprintf("multi_device_merge_winner_%s", winnerDevice),
	}

	// Log conflict for audit
	if err := cr.syncer.logConflict(conflict); err != nil {
		// Non-fatal
	}

	return merged, nil
}

// mergeAllVersions merges unique tags and metadata from all versions into winner
func (cr *ConflictResolver) mergeAllVersions(winner *TieredMemoryEntry, versions map[string]*TieredMemoryEntry) *TieredMemoryEntry {
	// Merge tags
	tagSet := make(map[string]bool)
	for _, t := range winner.Tags {
		tagSet[t] = true
	}

	for deviceID, version := range versions {
		if deviceID == "" {
			continue
		}
		for _, t := range version.Tags {
			if !tagSet[t] {
				winner.Tags = append(winner.Tags, t)
				tagSet[t] = true
			}
		}

		// Merge device-specific metadata
		if version.RemoteCommitHash != "" && winner.RemoteCommitHash == "" {
			winner.RemoteCommitHash = version.RemoteCommitHash
		}
	}

	return winner
}
