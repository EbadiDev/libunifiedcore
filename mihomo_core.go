package libunifiedcore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/hub/executor"
	mihomolog "github.com/metacubex/mihomo/log"
	"gopkg.in/yaml.v3"
)

// MihomoCoreManager manages Mihomo core instances using the proper hub.Parse approach
type MihomoCoreManager struct {
	mu        sync.RWMutex
	isRunning bool
	cancel    context.CancelFunc
	ctx       context.Context

	// Network configuration
	socksPort int // 15491 for tun2socks
	apiPort   int // 15490 for dashboard

	// Configuration
	configPath string
	configDir  string
	assetPath  string
	logLevel   string
}

// NewMihomoCoreManager creates a new Mihomo core manager
func NewMihomoCoreManager(socksPort, apiPort int) *MihomoCoreManager {
	return &MihomoCoreManager{
		socksPort: socksPort,
		apiPort:   apiPort,
		logLevel:  "info",
	}
}

// SetAssetPath sets the asset directory path
func (m *MihomoCoreManager) SetAssetPath(assetPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.assetPath = assetPath
}

// SetLogLevel sets the logging level
func (m *MihomoCoreManager) SetLogLevel(logLevel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logLevel = logLevel
}

// SetConfigDir sets the configuration directory
func (m *MihomoCoreManager) SetConfigDir(configDir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configDir = configDir
}

// GetConfigDir returns the configuration directory
func (m *MihomoCoreManager) GetConfigDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configDir
}

// RunConfig starts the Mihomo core with the specified configuration
func (m *MihomoCoreManager) RunConfig(configPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return fmt.Errorf("Mihomo core is already running")
	}

	m.configPath = configPath

	// Set up Mihomo environment
	if err := m.setupEnvironment(); err != nil {
		return fmt.Errorf("failed to setup environment: %w", err)
	}

	// Read and inject configuration
	configBytes, err := m.prepareConfigBytes(configPath)
	if err != nil {
		return fmt.Errorf("failed to prepare config: %w", err)
	}

	// Create context for cancellation
	m.ctx, m.cancel = context.WithCancel(context.Background())

	// Start core in goroutine
	go m.runCoreAsync(configBytes)

	// Wait a bit to ensure startup
	time.Sleep(300 * time.Millisecond)

	m.isRunning = true
	mihomolog.Infoln("Mihomo core started successfully on Mixed port %d, API port %d", m.socksPort, m.apiPort)
	return nil
}

// setupEnvironment sets up the Mihomo environment directories and paths
func (m *MihomoCoreManager) setupEnvironment() error {
	// Set home directory
	homeDir := m.assetPath
	if homeDir == "" {
		homeDir = m.configDir
	}
	if homeDir == "" {
		// Use current directory as fallback
		currentDir, _ := os.Getwd()
		homeDir = currentDir
	}

	// Ensure directory exists
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		return fmt.Errorf("failed to create home directory: %w", err)
	}

	// Set Mihomo paths
	C.SetHomeDir(homeDir)

	// Set config path (Mihomo expects this to be set)
	configFileName := "config.yaml"
	if m.configPath != "" {
		configFileName = filepath.Base(m.configPath)
	}
	C.SetConfig(filepath.Join(homeDir, configFileName))

	// Initialize config directory (this must succeed for logging to work)
	if err := config.Init(homeDir); err != nil {
		return fmt.Errorf("failed to initialize config directory: %w", err)
	}

	// Create log directory to ensure Mihomo can write logs
	logDir := filepath.Join(homeDir, "log")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	return nil
}

// prepareConfigBytes reads the config file, injects required settings, and returns bytes
func (m *MihomoCoreManager) prepareConfigBytes(configPath string) ([]byte, error) {
	// Read original config
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Try to parse as YAML first, then JSON
	var config map[string]interface{}
	if err := yaml.Unmarshal(configBytes, &config); err != nil {
		// Try JSON if YAML fails
		if err := json.Unmarshal(configBytes, &config); err != nil {
			return nil, fmt.Errorf("failed to parse config as YAML or JSON: %w", err)
		}
	}

	// Inject required configuration for tun2socks compatibility
	m.injectRequiredConfig(config)

	// Marshal back to YAML (Mihomo prefers YAML)
	injectedBytes, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal injected config: %w", err)
	}

	return injectedBytes, nil
}

// injectRequiredConfig injects SOCKS proxy and API configuration
func (m *MihomoCoreManager) injectRequiredConfig(config map[string]interface{}) {
	// Set mixed port for SOCKS proxy (tun2socks compatibility)
	config["mixed-port"] = m.socksPort
	config["bind-address"] = "127.0.0.1"

	// Set external controller for API
	config["external-controller"] = fmt.Sprintf("127.0.0.1:%d", m.apiPort)

	// Security settings
	config["allow-lan"] = false
	config["log-level"] = m.logLevel

	// Set mode to rule if not specified
	if _, exists := config["mode"]; !exists {
		config["mode"] = "rule"
	}

	// Ensure basic proxy structure exists if missing
	if _, exists := config["proxies"]; !exists {
		config["proxies"] = []interface{}{
			map[string]interface{}{
				"name": "DIRECT",
				"type": "direct",
			},
		}
	}

	// Ensure proxy-groups exist if missing
	if _, exists := config["proxy-groups"]; !exists {
		config["proxy-groups"] = []interface{}{
			map[string]interface{}{
				"name":    "PROXY",
				"type":    "select",
				"proxies": []string{"DIRECT"},
			},
		}
	}

	// Ensure rules exist if missing
	if _, exists := config["rules"]; !exists {
		config["rules"] = []interface{}{
			"MATCH,PROXY",
		}
	}

	// DNS configuration
	if _, exists := config["dns"]; !exists {
		config["dns"] = map[string]interface{}{
			"enable":             true,
			"listen":             "127.0.0.1:1053",
			"default-nameserver": []string{"8.8.8.8", "1.1.1.1"},
			"nameserver":         []string{"8.8.8.8", "1.1.1.1"},
		}
	}

	// Add log-file configuration if Flutter didn't inject it
	if _, exists := config["log-file"]; !exists {
		// Use same path structure as Flutter: applicationSupportDirectory/log/core.log
		logDir := filepath.Join(m.assetPath, "log")
		if m.assetPath == "" && m.configDir != "" {
			logDir = filepath.Join(m.configDir, "log")
		}
		if logDir != "" {
			os.MkdirAll(logDir, 0755) // Ensure log directory exists
			logFile := filepath.Join(logDir, "core.log")
			config["log-file"] = logFile
			mihomolog.Infoln("Added fallback log-file: %s", logFile)
		}
	} else {
		if logFile, ok := config["log-file"].(string); ok {
			mihomolog.Infoln("Using Flutter-injected log-file: %s", logFile)
			// Ensure the log directory exists for the injected path
			logDir := filepath.Dir(logFile)
			if err := os.MkdirAll(logDir, 0755); err != nil {
				mihomolog.Warnln("Failed to create log directory %s: %v", logDir, err)
			}
		} else {
			mihomolog.Infoln("Using Flutter-injected log-file configuration (non-string type)")
		}
	}

	mihomolog.Infoln("Mihomo config injected - Mixed port: %d, External controller: 127.0.0.1:%d", m.socksPort, m.apiPort)
}

// runCoreAsync runs the core asynchronously using hub.Parse (like Clash.Meta)
func (m *MihomoCoreManager) runCoreAsync(configBytes []byte) {
	defer func() {
		if r := recover(); r != nil {
			mihomolog.Errorln("Mihomo core panic recovered: %v", r)
		}
		m.mu.Lock()
		m.isRunning = false
		m.mu.Unlock()
	}()

	// Parse configuration using FlClash pattern
	rawConfig, err := config.UnmarshalRawConfig(configBytes)
	if err != nil {
		mihomolog.Errorln("Mihomo config.UnmarshalRawConfig error: %s", err.Error())
		return
	}

	parsedConfig, err := config.ParseRawConfig(rawConfig)
	if err != nil {
		mihomolog.Errorln("Mihomo config.ParseRawConfig error: %s", err.Error())
		return
	}

	// Apply configuration using FlClash pattern (hub.ApplyConfig instead of hub.Parse)
	hub.ApplyConfig(parsedConfig)

	mihomolog.Infoln("Mihomo core started successfully via hub.ApplyConfig")

	// Wait for shutdown signal
	<-m.ctx.Done()

	// Cleanup using executor.Shutdown (like Clash.Meta)
	executor.Shutdown()
	mihomolog.Infoln("Mihomo core stopped")
}

// Stop stops the Mihomo core
func (m *MihomoCoreManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return nil // Already stopped
	}

	// Cancel context to signal shutdown
	if m.cancel != nil {
		m.cancel()
	}

	// Give it time to cleanup
	time.Sleep(100 * time.Millisecond)

	m.isRunning = false
	mihomolog.Infoln("Mihomo core stopped successfully")
	return nil
}

// IsRunning returns whether the Mihomo core is running
func (m *MihomoCoreManager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isRunning
}

// TestConfig validates the Mihomo configuration without starting
func (m *MihomoCoreManager) TestConfig(configPath string) error {
	// Set up environment for testing
	if err := m.setupEnvironment(); err != nil {
		return fmt.Errorf("failed to setup environment: %w", err)
	}

	// Prepare config bytes
	configBytes, err := m.prepareConfigBytes(configPath)
	if err != nil {
		return fmt.Errorf("failed to prepare config: %w", err)
	}

	// Try to parse the configuration using Mihomo's parser
	if _, err := executor.ParseWithBytes(configBytes); err != nil {
		return fmt.Errorf("invalid Mihomo configuration: %w", err)
	}

	mihomolog.Infoln("Mihomo configuration validation passed: %s", configPath)
	return nil
}

// GetStats returns Mihomo specific statistics
func (m *MihomoCoreManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"core_type":   "mihomo",
		"running":     m.isRunning,
		"mixed_port":  m.socksPort,
		"api_port":    m.apiPort,
		"config_path": m.configPath,
		"asset_path":  m.assetPath,
		"config_dir":  m.configDir,
		"log_level":   m.logLevel,
	}
}

// UpdateConfig updates the configuration by restarting with new config
func (m *MihomoCoreManager) UpdateConfig(configPath string) error {
	if !m.isRunning {
		return fmt.Errorf("Mihomo core is not running")
	}

	// For Mihomo, we need to restart to apply new config
	mihomolog.Infoln("Restarting Mihomo core with new configuration...")

	if err := m.Stop(); err != nil {
		return fmt.Errorf("failed to stop core: %w", err)
	}

	// Wait for complete shutdown
	time.Sleep(200 * time.Millisecond)

	if err := m.RunConfig(configPath); err != nil {
		return fmt.Errorf("failed to start with new config: %w", err)
	}

	mihomolog.Infoln("Mihomo configuration updated successfully: %s", configPath)
	return nil
}
