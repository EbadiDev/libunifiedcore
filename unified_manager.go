package libunifiedcore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
)

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type UnifiedCoreManager struct {
	mu       sync.RWMutex
	coreType CoreType
	running  bool
	cancel   context.CancelFunc
	ctx      context.Context

	v2rayManager  *V2RayCoreManager
	mihomoManager *MihomoCoreManager

	socksPort int
	apiPort   int

	configPath   string
	configFormat string

	assetPath string
	logLevel  string
}

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
	u.configFormat = "json" // Always use JSON format

	log.Printf("Core type set to: %s", coreType.DisplayName())
	return nil
}

func (u *UnifiedCoreManager) SetCoreType(coreTypeStr string) error {
	coreType, err := ParseCoreType(coreTypeStr)
	if err != nil {
		return fmt.Errorf("failed to parse core type: %w", err)
	}
	return u.setCoreType(coreType)
}

func (u *UnifiedCoreManager) SetCoreTypeFromString(coreTypeStr string) error {
	coreType, err := ParseCoreType(coreTypeStr)
	if err != nil {
		return fmt.Errorf("failed to parse core type: %w", err)
	}
	return u.setCoreType(coreType)
}


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

func (u *UnifiedCoreManager) SetAssetPath(assetPath string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.assetPath = assetPath
}

func (u *UnifiedCoreManager) SetLogLevel(logLevel string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.logLevel = logLevel
}

func (u *UnifiedCoreManager) RunConfig(configPath string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.configPath = configPath

	log.Printf("Starting core with initial type: %s", u.coreType.DisplayName())

	// Always read coreType from Flutter's injected config
	configBytes, readErr := os.ReadFile(configPath)
	if readErr != nil {
		return fmt.Errorf("failed to read config file: %w", readErr)
	}

	log.Printf("Config file content preview: %s", string(configBytes[:minInt(200, len(configBytes))]))

	// Parse the injected config (must be JSON with coreType field)
	var injectedConfig map[string]interface{}
 	if err := json.Unmarshal(configBytes, &injectedConfig); err != nil {
		return fmt.Errorf("failed to parse injected config as JSON: %w", err)
	}

	// Read coreType field that Flutter must inject
	coreTypeStr, exists := injectedConfig["coreType"].(string)
	if !exists {
		return fmt.Errorf("injected config missing required coreType field - Flutter injection failed")
	}

	detectedCoreType, parseErr := ParseCoreType(coreTypeStr)
	if parseErr != nil {
		return fmt.Errorf("invalid coreType in injected config: %s - %w", coreTypeStr, parseErr)
	}

	// Check if we need to switch core types
	if u.running && u.coreType != detectedCoreType {
		log.Printf("Core type change detected: %s -> %s, stopping current core first", u.coreType.DisplayName(), detectedCoreType.DisplayName())
		
		// Stop the current running core
		var stopErr error
		switch u.coreType {
		case CoreTypeV2Ray, CoreTypeXray:
			stopErr = u.stopV2RayCore()
		case CoreTypeMihomo:
			stopErr = u.stopMihomoCore()
			// Give Mihomo cores extra time to cleanup goroutines and channels
			time.Sleep(100 * time.Millisecond)
		}

		if u.cancel != nil {
			u.cancel()
			u.cancel = nil
		}

		u.running = false

		if stopErr != nil {
			log.Printf("Warning: Failed to stop previous %s core: %v", u.coreType.DisplayName(), stopErr)
		}

		// Brief wait for port cleanup - VPN apps need speed
		time.Sleep(50 * time.Millisecond)
	}

	u.coreType = detectedCoreType
	u.configFormat = "json" // Always use JSON format
	log.Printf("Using core type from injected config: %s", detectedCoreType.DisplayName())

	// If already running the same core type, stop it first to restart with new config
	if u.running {
		log.Printf("Core already running, stopping first to restart with new config")
		
		var stopErr error
		switch u.coreType {
		case CoreTypeV2Ray, CoreTypeXray:
			stopErr = u.stopV2RayCore()
		case CoreTypeMihomo:
			stopErr = u.stopMihomoCore()
			// Give Mihomo cores extra time to cleanup
			time.Sleep(100 * time.Millisecond)
		}

		if u.cancel != nil {
			u.cancel()
			u.cancel = nil
		}

		u.running = false

		if stopErr != nil {
			log.Printf("Warning: Failed to stop core for restart: %v", stopErr)
		}

		// Brief wait for cleanup
		time.Sleep(50 * time.Millisecond)
	}

	// Extract ports from Flutter's injected config instead of generating random ones
	if socksPortRaw, exists := injectedConfig["mixed-port"]; exists {
		if socksPortFloat, ok := socksPortRaw.(float64); ok {
			u.socksPort = int(socksPortFloat)
		}
	}
	if apiPortRaw, exists := injectedConfig["external-controller"]; exists {
		if apiPortStr, ok := apiPortRaw.(string); ok {
			// Parse "127.0.0.1:port" format
			colonIndex := -1
			for i := len(apiPortStr) - 1; i >= 0; i-- {
				if apiPortStr[i] == ':' {
					colonIndex = i
					break
				}
			}
			if colonIndex >= 0 && colonIndex < len(apiPortStr)-1 {
				portStr := apiPortStr[colonIndex+1:]
				if port, parseErr := strconv.Atoi(portStr); parseErr == nil {
					u.apiPort = port
				}
			}
		}
	}
	
	// Fallback to random ports if not found in config
	if u.socksPort == 0 {
		u.socksPort = 10000 + time.Now().Nanosecond()%50000
	}
	if u.apiPort == 0 {
		u.apiPort = 10000 + time.Now().Nanosecond()%50000
	}
	log.Printf("Final ports configured - SOCKS: %d, API: %d", u.socksPort, u.apiPort)

	u.ctx, u.cancel = context.WithCancel(context.Background())

	var err error
	switch u.coreType {
	case CoreTypeV2Ray, CoreTypeXray:
		err = u.startV2RayCore(configPath)
	case CoreTypeMihomo:
		err = u.startMihomoCore(configPath)
		// For bulk ping tests, ensure Mihomo core has time to stabilize
		if err == nil {
			time.Sleep(50 * time.Millisecond)
		}
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

func (u *UnifiedCoreManager) Stop() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if !u.running {
		return nil
	}

	var err error
	switch u.coreType {
	case CoreTypeV2Ray, CoreTypeXray:
		err = u.stopV2RayCore()
	case CoreTypeMihomo:
		err = u.stopMihomoCore()
		// Allow extra time for Mihomo cleanup in bulk testing scenarios
		time.Sleep(50 * time.Millisecond)
	}

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

func (u *UnifiedCoreManager) IsRunning() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.running
}

func (u *UnifiedCoreManager) GetCoreType() CoreType {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.coreType
}

func (u *UnifiedCoreManager) GetCoreTypeString() string {
	return u.GetCoreType().String()
}

func (u *UnifiedCoreManager) GetSOCKSPort() int {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.socksPort
}

func (u *UnifiedCoreManager) GetAPIPort() int {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.apiPort
}

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

	time.Sleep(100 * time.Millisecond)

	return u.RunConfig(configPath)
}

func (u *UnifiedCoreManager) SwitchCoreType(newCoreType CoreType) error {
	u.mu.RLock()
	currentlyRunning := u.running
	configPath := u.configPath
	u.mu.RUnlock()

	if currentlyRunning {
		if err := u.Stop(); err != nil {
			return fmt.Errorf("failed to stop current core: %w", err)
		}
	}

	if err := u.setCoreType(newCoreType); err != nil {
		return fmt.Errorf("failed to set new core type: %w", err)
	}

	if currentlyRunning && configPath != "" {
		if err := u.RunConfig(configPath); err != nil {
			return fmt.Errorf("failed to start new core: %w", err)
		}
	}

	return nil
}

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

func (u *UnifiedCoreManager) startV2RayCore(configPath string) error {
	if globalV2RayManager == nil {
		globalV2RayManager = NewV2RayCoreManager(u.socksPort, u.apiPort)
	} else {
		// Update ports for this test
		globalV2RayManager.socksPort = u.socksPort
		globalV2RayManager.apiPort = u.apiPort
	}
	globalV2RayManager.SetAssetPath(u.assetPath)
	globalV2RayManager.SetLogLevel(u.logLevel)
	
	u.v2rayManager = globalV2RayManager
	return u.v2rayManager.RunConfig(configPath)
}

func (u *UnifiedCoreManager) stopV2RayCore() error {
	if u.v2rayManager != nil {
		return u.v2rayManager.Stop()
	}
	return nil
}

func (u *UnifiedCoreManager) testV2RayConfig(configPath string) error {
	if globalV2RayManager == nil {
		globalV2RayManager = NewV2RayCoreManager(u.socksPort, u.apiPort)
	}
	return globalV2RayManager.TestConfig(configPath)
}

func (u *UnifiedCoreManager) startMihomoCore(configPath string) error {
	if globalMihomoManager == nil {
		globalMihomoManager = NewMihomoCoreManager(u.socksPort, u.apiPort)
	} else {
		// Update ports for this test
		globalMihomoManager.socksPort = u.socksPort
		globalMihomoManager.apiPort = u.apiPort
	}
	globalMihomoManager.SetAssetPath(u.assetPath)
	globalMihomoManager.SetLogLevel(u.logLevel)
	
	u.mihomoManager = globalMihomoManager
	return u.mihomoManager.RunConfig(configPath)
}

func (u *UnifiedCoreManager) stopMihomoCore() error {
	if u.mihomoManager != nil {
		return u.mihomoManager.Stop()
	}
	return nil
}

func (u *UnifiedCoreManager) testMihomoConfig(configPath string) error {
	if globalMihomoManager == nil {
		globalMihomoManager = NewMihomoCoreManager(u.socksPort, u.apiPort)
	}
	return globalMihomoManager.TestConfig(configPath)
}
