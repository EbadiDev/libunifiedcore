package libunifiedcore

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// UnifiedCoreManager manages multiple proxy cores in a unified interface
type UnifiedCoreManager struct {
	mu       sync.RWMutex
	coreType CoreType
	running  bool
	cancel   context.CancelFunc
	ctx      context.Context

	// Core implementations
	v2rayManager  *V2RayCoreManager
	mihomoManager *MihomoCoreManager

	// Network configuration
	socksPort int // 15491 for tun2socks
	apiPort   int // 15490 for dashboard

	// Configuration
	configPath   string
	configFormat string

	// Environment settings
	assetPath string
	logLevel  string
}

// setCoreType sets the core type for the manager (internal method)
func (u *UnifiedCoreManager) setCoreType(coreType CoreType) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.running {
		return fmt.Errorf("cannot change core type while running")
	}

	if !coreType.IsValid() {
		return fmt.Errorf("invalid core type: %v", coreType)
	}

	u.coreType = coreType
	u.configFormat = coreType.GetConfigFormat()

	log.Printf("Core type set to: %s", coreType.DisplayName())
	return nil
}

// SetCoreType sets the core type from a string (gomobile compatible)
func (u *UnifiedCoreManager) SetCoreType(coreTypeStr string) error {
	coreType, err := ParseCoreType(coreTypeStr)
	if err != nil {
		return fmt.Errorf("failed to parse core type: %w", err)
	}
	return u.setCoreType(coreType)
}

// SetCoreTypeFromString sets the core type from a string
func (u *UnifiedCoreManager) SetCoreTypeFromString(coreTypeStr string) error {
	coreType, err := ParseCoreType(coreTypeStr)
	if err != nil {
		return fmt.Errorf("failed to parse core type: %w", err)
	}
	return u.setCoreType(coreType)
}

// DetectAndSetCoreType detects core type from configuration content
func (u *UnifiedCoreManager) DetectAndSetCoreType(configContent string) error {
	coreType, err := DetectCoreTypeFromConfig(configContent)
	if err != nil {
		return fmt.Errorf("failed to detect core type: %w", err)
	}
	return u.setCoreType(coreType)
}

// SetPorts configures the SOCKS and API ports
func (u *UnifiedCoreManager) SetPorts(socksPort, apiPort int) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.running {
		return fmt.Errorf("cannot change ports while running")
	}

	if socksPort <= 0 || socksPort > 65535 {
		return fmt.Errorf("invalid SOCKS port: %d", socksPort)
	}

	if apiPort <= 0 || apiPort > 65535 {
		return fmt.Errorf("invalid API port: %d", apiPort)
	}

	u.socksPort = socksPort
	u.apiPort = apiPort

	log.Printf("Ports configured - SOCKS: %d, API: %d", socksPort, apiPort)
	return nil
}

// SetAssetPath sets the asset directory path for cores
func (u *UnifiedCoreManager) SetAssetPath(assetPath string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.assetPath = assetPath
}

// SetLogLevel sets the logging level
func (u *UnifiedCoreManager) SetLogLevel(logLevel string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.logLevel = logLevel
}

// RunConfig starts the core with the specified configuration file
func (u *UnifiedCoreManager) RunConfig(configPath string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.running {
		return fmt.Errorf("core is already running")
	}

	u.configPath = configPath

	// Create context for cancellation
	u.ctx, u.cancel = context.WithCancel(context.Background())

	var err error
	switch u.coreType {
	case CoreTypeV2Ray, CoreTypeXray:
		err = u.startV2RayCore(configPath)
	case CoreTypeMihomo:
		err = u.startMihomoCore(configPath)
	default:
		return fmt.Errorf("unsupported core type: %v", u.coreType)
	}

	if err != nil {
		if u.cancel != nil {
			u.cancel()
		}
		return fmt.Errorf("failed to start %s core: %w", u.coreType.DisplayName(), err)
	}

	u.running = true
	log.Printf("%s core started successfully with config: %s", u.coreType.DisplayName(), configPath)
	return nil
}

// Stop stops the currently running core
func (u *UnifiedCoreManager) Stop() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if !u.running {
		return nil // Already stopped
	}

	var err error
	switch u.coreType {
	case CoreTypeV2Ray, CoreTypeXray:
		err = u.stopV2RayCore()
	case CoreTypeMihomo:
		err = u.stopMihomoCore()
	}

	// Cancel context
	if u.cancel != nil {
		u.cancel()
		u.cancel = nil
	}

	u.running = false
	u.configPath = ""

	if err != nil {
		log.Printf("Error stopping %s core: %v", u.coreType.DisplayName(), err)
		return err
	}

	log.Printf("%s core stopped successfully", u.coreType.DisplayName())
	return nil
}

// IsRunning returns whether the core is currently running
func (u *UnifiedCoreManager) IsRunning() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.running
}

// GetCoreType returns the current core type
func (u *UnifiedCoreManager) GetCoreType() CoreType {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.coreType
}

// GetCoreTypeString returns the current core type as a string
func (u *UnifiedCoreManager) GetCoreTypeString() string {
	return u.GetCoreType().String()
}

// GetSOCKSPort returns the configured SOCKS port
func (u *UnifiedCoreManager) GetSOCKSPort() int {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.socksPort
}

// GetAPIPort returns the configured API port
func (u *UnifiedCoreManager) GetAPIPort() int {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.apiPort
}

// TestConfig validates the configuration without starting the core
func (u *UnifiedCoreManager) TestConfig(configPath string) error {
	u.mu.RLock()
	coreType := u.coreType
	u.mu.RUnlock()

	switch coreType {
	case CoreTypeV2Ray, CoreTypeXray:
		return u.testV2RayConfig(configPath)
	case CoreTypeMihomo:
		return u.testMihomoConfig(configPath)
	default:
		return fmt.Errorf("unsupported core type for testing: %v", coreType)
	}
}

// Restart stops and restarts the core with the current configuration
func (u *UnifiedCoreManager) Restart() error {
	u.mu.RLock()
	configPath := u.configPath
	u.mu.RUnlock()

	if configPath == "" {
		return fmt.Errorf("no configuration path set")
	}

	if err := u.Stop(); err != nil {
		return fmt.Errorf("failed to stop core for restart: %w", err)
	}

	// Wait a bit for proper cleanup
	time.Sleep(100 * time.Millisecond)

	return u.RunConfig(configPath)
}

// SwitchCoreType switches to a different core type and restarts if running
func (u *UnifiedCoreManager) SwitchCoreType(newCoreType CoreType) error {
	u.mu.RLock()
	currentlyRunning := u.running
	configPath := u.configPath
	u.mu.RUnlock()

	// Stop current core if running
	if currentlyRunning {
		if err := u.Stop(); err != nil {
			return fmt.Errorf("failed to stop current core: %w", err)
		}
	}

	// Set new core type
	if err := u.setCoreType(newCoreType); err != nil {
		return fmt.Errorf("failed to set new core type: %w", err)
	}

	// Restart with previous config if it was running
	if currentlyRunning && configPath != "" {
		if err := u.RunConfig(configPath); err != nil {
			return fmt.Errorf("failed to start new core: %w", err)
		}
	}

	return nil
}

// GetStats returns basic statistics about the core
func (u *UnifiedCoreManager) GetStats() map[string]interface{} {
	u.mu.RLock()
	defer u.mu.RUnlock()

	stats := map[string]interface{}{
		"core_type":     u.coreType.String(),
		"core_name":     u.coreType.DisplayName(),
		"running":       u.running,
		"socks_port":    u.socksPort,
		"api_port":      u.apiPort,
		"config_path":   u.configPath,
		"config_format": u.configFormat,
	}

	// Add core-specific stats
	switch u.coreType {
	case CoreTypeV2Ray, CoreTypeXray:
		if u.v2rayManager != nil {
			stats["v2ray_running"] = u.v2rayManager.IsRunning()
		}
	case CoreTypeMihomo:
		if u.mihomoManager != nil {
			stats["mihomo_running"] = u.mihomoManager.IsRunning()
		}
	}

	return stats
}

// Core-specific implementation methods (to be implemented in separate files)

func (u *UnifiedCoreManager) startV2RayCore(configPath string) error {
	if u.v2rayManager == nil {
		u.v2rayManager = NewV2RayCoreManager(u.socksPort, u.apiPort)
		u.v2rayManager.SetAssetPath(u.assetPath)
		u.v2rayManager.SetLogLevel(u.logLevel)
	}
	return u.v2rayManager.RunConfig(configPath)
}

func (u *UnifiedCoreManager) stopV2RayCore() error {
	if u.v2rayManager != nil {
		return u.v2rayManager.Stop()
	}
	return nil
}

func (u *UnifiedCoreManager) testV2RayConfig(configPath string) error {
	if u.v2rayManager == nil {
		u.v2rayManager = NewV2RayCoreManager(u.socksPort, u.apiPort)
	}
	return u.v2rayManager.TestConfig(configPath)
}

func (u *UnifiedCoreManager) startMihomoCore(configPath string) error {
	if u.mihomoManager == nil {
		u.mihomoManager = NewMihomoCoreManager(u.socksPort, u.apiPort)
		u.mihomoManager.SetAssetPath(u.assetPath)
		u.mihomoManager.SetLogLevel(u.logLevel)
	}
	return u.mihomoManager.RunConfig(configPath)
}

func (u *UnifiedCoreManager) stopMihomoCore() error {
	if u.mihomoManager != nil {
		return u.mihomoManager.Stop()
	}
	return nil
}

func (u *UnifiedCoreManager) testMihomoConfig(configPath string) error {
	if u.mihomoManager == nil {
		u.mihomoManager = NewMihomoCoreManager(u.socksPort, u.apiPort)
	}
	return u.mihomoManager.TestConfig(configPath)
}
