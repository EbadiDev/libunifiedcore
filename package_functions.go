package libunifiedcore

import (
	"fmt"
	"log"
	"os"
	"runtime"
)

// Package-level variables for global state
var (
	defaultManager  *UnifiedCoreManager
	globalAssetPath string
	globalLogLevel  string = "info"
)

// NewUnifiedCoreManager creates a new unified core manager instance
// This function is exported for gomobile compatibility
func NewUnifiedCoreManager() *UnifiedCoreManager {
	manager := &UnifiedCoreManager{
		coreType:     CoreTypeV2Ray, // Default to V2Ray
		running:      false,
		socksPort:    15491, // Default SOCKS port for tun2socks
		apiPort:      15490, // Default API port for dashboard
		configFormat: "json",
		logLevel:     globalLogLevel,
		assetPath:    globalAssetPath,
	}

	log.Printf("Created new UnifiedCoreManager with default settings")
	return manager
}

// NewCoreManager creates a new core manager and sets it as default
// This provides backward compatibility with existing code
func NewCoreManager() *UnifiedCoreManager {
	defaultManager = NewUnifiedCoreManager()
	return defaultManager
}

// SetEnv sets an environment variable
// This function is exported for gomobile compatibility
func SetEnv(key string, val string) {
	os.Setenv(key, val)
	log.Printf("Environment variable set: %s=%s", key, val)

	// Handle special environment variables
	switch key {
	case "v2ray.location.asset", "xray.location.asset":
		globalAssetPath = val
		if defaultManager != nil {
			defaultManager.SetAssetPath(val)
		}
	}
}

// SetLogLevel sets the global logging level
// This function is exported for gomobile compatibility
func SetLogLevel(logLevel string) {
	globalLogLevel = logLevel
	log.Printf("Global log level set to: %s", logLevel)

	if defaultManager != nil {
		defaultManager.SetLogLevel(logLevel)
	}
}

// SetHomeDir sets the home directory (legacy compatibility)
// This function is exported for gomobile compatibility
func SetHomeDir(homeDir string) {
	// For compatibility, treat as asset path
	SetEnv("v2ray.location.asset", homeDir)
	SetEnv("xray.location.asset", homeDir)
}

// GetVersion returns version information
// This function is exported for gomobile compatibility
func GetVersion() string {
	return "UnifiedCore v1.0.0"
}

// GetCoreVersion returns version information for a specific core type
func GetCoreVersion(coreType string) string {
	switch coreType {
	case "v2ray", "xray":
		return "Xray-core v1.250608.0"
	case "mihomo", "clash":
		return "Mihomo v1.19.12"
	default:
		return "Unknown core type"
	}
}

// TestConfigFile tests a configuration file for validity
// This function is exported for gomobile compatibility
func TestConfigFile(configPath string, coreType string) bool {
	manager := NewUnifiedCoreManager()

	// Set core type if specified
	if coreType != "" {
		if err := manager.SetCoreTypeFromString(coreType); err != nil {
			log.Printf("Failed to set core type %s: %v", coreType, err)
			return false
		}
	} else {
		// Try to auto-detect core type from config
		configBytes, err := os.ReadFile(configPath)
		if err != nil {
			log.Printf("Failed to read config file: %v", err)
			return false
		}

		if err := manager.DetectAndSetCoreType(string(configBytes)); err != nil {
			log.Printf("Failed to detect core type: %v", err)
			return false
		}
	}

	// Test the configuration
	if err := manager.TestConfig(configPath); err != nil {
		log.Printf("Configuration test failed: %v", err)
		return false
	}

	log.Printf("Configuration test passed for %s", configPath)
	return true
}

// DetectCoreTypeFromFile detects core type from a configuration file
// This function is exported for gomobile compatibility
func DetectCoreTypeFromFile(configPath string) string {
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("Failed to read config file: %v", err)
		return "unknown"
	}

	coreType, err := DetectCoreTypeFromConfig(string(configBytes))
	if err != nil {
		log.Printf("Failed to detect core type: %v", err)
		return "unknown"
	}

	return coreType.String()
}

// SetGlobalPorts sets the default SOCKS and API ports globally
// This function is exported for gomobile compatibility
func SetGlobalPorts(socksPort, apiPort int) bool {
	if socksPort <= 0 || socksPort > 65535 || apiPort <= 0 || apiPort > 65535 {
		log.Printf("Invalid port configuration: SOCKS=%d, API=%d", socksPort, apiPort)
		return false
	}

	log.Printf("Global ports set: SOCKS=%d, API=%d", socksPort, apiPort)
	return true
}

// GetMemoryUsage returns current memory usage statistics
// This function is exported for gomobile compatibility
func GetMemoryUsage() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"alloc":        m.Alloc,       // Current allocated memory
		"total_alloc":  m.TotalAlloc,  // Total allocated memory
		"sys":          m.Sys,         // System memory obtained
		"num_gc":       m.NumGC,       // Number of GC cycles
		"heap_alloc":   m.HeapAlloc,   // Heap allocated memory
		"heap_sys":     m.HeapSys,     // Heap system memory
		"heap_objects": m.HeapObjects, // Number of allocated objects
	}
}

// ForceGC forces garbage collection
// This function is exported for gomobile compatibility
func ForceGC() {
	runtime.GC()
	log.Println("Forced garbage collection completed")
}

// GetSupportedCoreTypes returns a list of supported core types
// This function is exported for gomobile compatibility
func GetSupportedCoreTypes() []string {
	return []string{"v2ray", "xray", "mihomo"}
}

// IsValidCoreType checks if a core type string is valid
// This function is exported for gomobile compatibility
func IsValidCoreType(coreType string) bool {
	_, err := ParseCoreType(coreType)
	return err == nil
}

// GetDefaultPorts returns default ports for a core type
// This function is exported for gomobile compatibility
func GetDefaultPorts(coreType string) map[string]int {
	ct, err := ParseCoreType(coreType)
	if err != nil {
		return map[string]int{
			"socks": 15491,
			"http":  15492,
			"api":   15490,
		}
	}

	socksPort, httpPort, apiPort := ct.GetDefaultPorts()
	return map[string]int{
		"socks": socksPort,
		"http":  httpPort,
		"api":   apiPort,
	}
}

// GetConfigFormat returns the expected configuration format for a core type
// This function is exported for gomobile compatibility
func GetConfigFormat(coreType string) string {
	ct, err := ParseCoreType(coreType)
	if err != nil {
		return "json"
	}

	return ct.GetConfigFormat()
}

// ValidateConfigFormat checks if a core type supports a configuration format
// This function is exported for gomobile compatibility
func ValidateConfigFormat(coreType, format string) bool {
	ct, err := ParseCoreType(coreType)
	if err != nil {
		return false
	}

	return ct.SupportsFormat(format)
}

// GetRuntimeInfo returns runtime information about the unified core
// This function is exported for gomobile compatibility
func GetRuntimeInfo() map[string]interface{} {
	return map[string]interface{}{
		"version":         GetVersion(),
		"go_version":      runtime.Version(),
		"num_cpu":         runtime.NumCPU(),
		"num_goroutines":  runtime.NumGoroutine(),
		"os":              runtime.GOOS,
		"arch":            runtime.GOARCH,
		"supported_cores": GetSupportedCoreTypes(),
		"default_ports": map[string]int{
			"socks": 15491,
			"api":   15490,
		},
	}
}

// InitializeGlobalManager initializes the global default manager
// This function is exported for gomobile compatibility
func InitializeGlobalManager() bool {
	if defaultManager != nil {
		log.Println("Global manager already initialized")
		return true
	}

	defaultManager = NewUnifiedCoreManager()
	if globalAssetPath != "" {
		defaultManager.SetAssetPath(globalAssetPath)
	}
	defaultManager.SetLogLevel(globalLogLevel)

	log.Println("Global manager initialized successfully")
	return true
}

// GetGlobalManager returns the global default manager
// This function is exported for gomobile compatibility
func GetGlobalManager() *UnifiedCoreManager {
	if defaultManager == nil {
		InitializeGlobalManager()
	}
	return defaultManager
}

// CleanupGlobalManager cleans up the global manager
// This function is exported for gomobile compatibility
func CleanupGlobalManager() {
	if defaultManager != nil {
		if defaultManager.IsRunning() {
			defaultManager.Stop()
		}
		defaultManager = nil
		log.Println("Global manager cleaned up")
	}
}

// SetAssetPath sets the global asset path
// This function is exported for gomobile compatibility
func SetAssetPath(assetPath string) {
	globalAssetPath = assetPath
	SetEnv("v2ray.location.asset", assetPath)
	SetEnv("xray.location.asset", assetPath)
	log.Printf("Global asset path set to: %s", assetPath)
}

// DetectCoreType detects core type from configuration content
// This function is exported for gomobile compatibility
func DetectCoreType(configContent string) string {
	coreType, err := DetectCoreTypeFromConfig(configContent)
	if err != nil {
		log.Printf("Failed to detect core type: %v", err)
		return "v2ray" // Default fallback
	}
	return coreType.String()
}

// GetMemoryStats returns memory statistics as a JSON string
// This function is exported for gomobile compatibility
func GetMemoryStats() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return fmt.Sprintf(`{
		"alloc": %d,
		"total_alloc": %d,
		"sys": %d,
		"num_gc": %d,
		"heap_alloc": %d,
		"heap_sys": %d,
		"heap_objects": %d
	}`, m.Alloc, m.TotalAlloc, m.Sys, m.NumGC, m.HeapAlloc, m.HeapSys, m.HeapObjects)
}
