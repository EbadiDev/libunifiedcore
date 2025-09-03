package libunifiedcore

import (
	"fmt"
	"log"
	"os"
	"runtime"
)

var (
	globalUnifiedManager *UnifiedCoreManager
	globalAssetPath      string
	globalLogLevel       string = "info"
	
	// Singleton instances to prevent conflicts
	globalMihomoManager *MihomoCoreManager
	globalV2RayManager  *V2RayCoreManager
)

func NewUnifiedCoreManager() *UnifiedCoreManager {
	// Create a new manager instance for each ping test (Flutter expects isolated managers)
	// but the underlying core managers will be singletons to prevent conflicts
	manager := &UnifiedCoreManager{
		coreType:     CoreTypeXray, // Default to Xray, will be detected from config
		running:      false,
		socksPort:    0, // Will be set from injected config
		apiPort:      0, // Will be set from injected config
		configFormat: "json",
		logLevel:     globalLogLevel,
		assetPath:    globalAssetPath,
	}

	log.Printf("Created new UnifiedCoreManager (isolated instance for ping test)")
	return manager
}

func NewCoreManager() *UnifiedCoreManager {
	return NewUnifiedCoreManager()
}

func SetEnv(key string, val string) {
	os.Setenv(key, val)
	log.Printf("Environment variable set: %s=%s", key, val)

	switch key {
	case "v2ray.location.asset", "xray.location.asset":
		globalAssetPath = val
		if globalUnifiedManager != nil {
			globalUnifiedManager.SetAssetPath(val)
		}
	}
}

func SetLogLevel(logLevel string) {
	globalLogLevel = logLevel
	log.Printf("Global log level set to: %s", logLevel)

	if globalUnifiedManager != nil {
		globalUnifiedManager.SetLogLevel(logLevel)
	}
}

func SetHomeDir(homeDir string) {
	SetEnv("v2ray.location.asset", homeDir)
	SetEnv("xray.location.asset", homeDir)
}

func GetVersion() string {
	return "UnifiedCore v1.0.0"
}

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

func TestConfigFile(configPath string, coreType string) bool {
	manager := NewUnifiedCoreManager()

	if coreType != "" {
		if err := manager.SetCoreTypeFromString(coreType); err != nil {
			log.Printf("Failed to set core type %s: %v", coreType, err)
			return false
		}
	} else {
		// Without explicit core type, we can't test the config since Flutter injection is required
		log.Printf("Core type must be specified for config testing")
		return false
	}

	if err := manager.TestConfig(configPath); err != nil {
		log.Printf("Configuration test failed: %v", err)
		return false
	}

	log.Printf("Configuration test passed for %s", configPath)
	return true
}



func SetGlobalPorts(socksPort, apiPort int) bool {
	if socksPort <= 0 || socksPort > 65535 || apiPort <= 0 || apiPort > 65535 {
		log.Printf("Invalid port configuration: SOCKS=%d, API=%d", socksPort, apiPort)
		return false
	}

	log.Printf("Global ports set: SOCKS=%d, API=%d", socksPort, apiPort)
	return true
}

func GetMemoryUsage() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"alloc":        m.Alloc,
		"total_alloc":  m.TotalAlloc,
		"sys":          m.Sys,
		"num_gc":       m.NumGC,
		"heap_alloc":   m.HeapAlloc,
		"heap_sys":     m.HeapSys,
		"heap_objects": m.HeapObjects,
	}
}

func ForceGC() {
	runtime.GC()
	log.Println("Forced garbage collection completed")
}

func GetSupportedCoreTypes() []string {
	return []string{"v2ray", "xray", "mihomo"}
}

func IsValidCoreType(coreType string) bool {
	_, err := ParseCoreType(coreType)
	return err == nil
}



func GetRuntimeInfo() map[string]interface{} {
	return map[string]interface{}{
		"version":         GetVersion(),
		"go_version":      runtime.Version(),
		"num_cpu":         runtime.NumCPU(),
		"num_goroutines":  runtime.NumGoroutine(),
		"os":              runtime.GOOS,
		"arch":            runtime.GOARCH,
		"supported_cores": GetSupportedCoreTypes(),
	}
}

func InitializeGlobalManager() bool {
	if globalUnifiedManager != nil {
		log.Println("Global unified manager already initialized")
		return true
	}

	globalUnifiedManager = NewUnifiedCoreManager()
	if globalAssetPath != "" {
		globalUnifiedManager.SetAssetPath(globalAssetPath)
	}
	globalUnifiedManager.SetLogLevel(globalLogLevel)

	log.Println("Global unified manager initialized successfully")
	return true
}

func GetGlobalManager() *UnifiedCoreManager {
	if globalUnifiedManager == nil {
		InitializeGlobalManager()
	}
	return globalUnifiedManager
}

func CleanupGlobalManager() {
	if globalUnifiedManager != nil {
		if globalUnifiedManager.IsRunning() {
			globalUnifiedManager.Stop()
		}
		globalUnifiedManager = nil
		log.Println("Global unified manager cleaned up")
	}
}

func SetAssetPath(assetPath string) {
	globalAssetPath = assetPath
	SetEnv("v2ray.location.asset", assetPath)
	SetEnv("xray.location.asset", assetPath)
	log.Printf("Global asset path set to: %s", assetPath)
}



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
