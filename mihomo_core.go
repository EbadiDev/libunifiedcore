package libunifiedcore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/metacubex/mihomo/common/observable"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/hub/executor"
	mihomolog "github.com/metacubex/mihomo/log"
	"gopkg.in/yaml.v3"
)

type MihomoCoreManager struct {
	mu        sync.RWMutex
	isRunning bool
	cancel    context.CancelFunc
	ctx       context.Context

	socksPort  int
	apiPort    int
	configPath string
	configDir  string
	assetPath  string
	logLevel   string

	logSubscriber observable.Subscription[mihomolog.Event]
	logFilePath   string
	
	// Add run lock to prevent race conditions like FlClash does
	runLock       sync.Mutex
}

func NewMihomoCoreManager(socksPort, apiPort int) *MihomoCoreManager {
	return &MihomoCoreManager{
		socksPort: socksPort,
		apiPort:   apiPort,
		logLevel:  "info",
	}
}

func (m *MihomoCoreManager) SetAssetPath(assetPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.assetPath = assetPath
}

func (m *MihomoCoreManager) SetLogLevel(logLevel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logLevel = logLevel
}

func (m *MihomoCoreManager) SetConfigDir(configDir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configDir = configDir
}

func (m *MihomoCoreManager) GetConfigDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configDir
}

func (m *MihomoCoreManager) RunConfig(configPath string) error {
	m.runLock.Lock()
	defer m.runLock.Unlock()
	
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return fmt.Errorf("mihomo core is already running")
	}

	m.configPath = configPath

	if err := m.setupEnvironment(); err != nil {
		return fmt.Errorf("failed to setup environment: %w", err)
	}

	configBytes, err := m.prepareConfigBytes(configPath)
	if err != nil {
		return fmt.Errorf("failed to prepare config: %w", err)
	}

	m.ctx, m.cancel = context.WithCancel(context.Background())

	go m.runCoreAsync(configBytes)

	// Wait a brief moment for core startup - Flutter already provides available ports
	time.Sleep(100 * time.Millisecond)

	m.isRunning = true
	mihomolog.Infoln("Mihomo core started successfully on Mixed port %d, API port %d", m.socksPort, m.apiPort)
	return nil
}

func (m *MihomoCoreManager) setupEnvironment() error {

	homeDir := m.assetPath
	if homeDir == "" {
		homeDir = m.configDir
	}
	if homeDir == "" {

		currentDir, _ := os.Getwd()
		homeDir = currentDir
	}

	if err := os.MkdirAll(homeDir, 0755); err != nil {
		return fmt.Errorf("failed to create home directory: %w", err)
	}

	C.SetHomeDir(homeDir)

	configFileName := "config.yaml"
	if m.configPath != "" {
		configFileName = filepath.Base(m.configPath)
	}
	C.SetConfig(filepath.Join(homeDir, configFileName))

	if err := config.Init(homeDir); err != nil {
		return fmt.Errorf("failed to initialize config directory: %w", err)
	}

	logDir := filepath.Join(homeDir, "log")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	return nil
}

func (m *MihomoCoreManager) prepareConfigBytes(configPath string) ([]byte, error) {
	jsonBytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// The config from Flutter is JSON. We need to convert it to YAML for mihomo.
	// We unmarshal to a generic interface{} to preserve data structures.
	var configData interface{}
	if err := json.Unmarshal(jsonBytes, &configData); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Marshal the Go data structure to YAML bytes.
	yamlBytes, err := yaml.Marshal(configData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	// For log subscription, we can peek into the map.
	if configMap, ok := configData.(map[string]interface{}); ok {
		if logFile, exists := configMap["log-file"]; exists {
			if logPath, ok := logFile.(string); ok {
				m.logFilePath = logPath
				mihomolog.Infoln("Extracted log file path from config: %s", m.logFilePath)
			} else {
				mihomolog.Warnln("log-file exists but is not a string: %v", logFile)
			}
		} else {
			mihomolog.Warnln("log-file field not found in config")
		}
	}

	mihomolog.Infoln("Using pre-injected Mihomo config from Flutter ConfigInjectorUnified")

	return yamlBytes, nil
}

func (m *MihomoCoreManager) runCoreAsync(configBytes []byte) {
	defer func() {
		if r := recover(); r != nil {
			mihomolog.Errorln("Mihomo core panicked: %v", r)
		}
	}()

	rawConfig, err := config.UnmarshalRawConfig(configBytes)
	if err != nil {
		mihomolog.Errorln("Failed to unmarshal Mihomo config: %v", err)
		return
	}

	parsedConfig, err := config.ParseRawConfig(rawConfig)
	if err != nil {
		mihomolog.Errorln("Failed to parse Mihomo config: %v", err)
		return
	}

	// Start log subscription BEFORE applying config to catch startup logs
	mihomolog.Infoln("About to call startLogSubscription with path: %s", m.logFilePath)
	m.startLogSubscription()
	mihomolog.Infoln("startLogSubscription call completed")

	// Apply config with proper error handling
	mihomolog.Infoln("Applying Mihomo configuration...")
	hub.ApplyConfig(parsedConfig)

	mihomolog.SetLevel(parsedConfig.General.LogLevel)
	mihomolog.Infoln("Mihomo: Log level set to: %s", parsedConfig.General.LogLevel.String())

	mihomolog.Infoln("Mihomo core started successfully via hub.ApplyConfig")

	// Wait for shutdown signal
	<-m.ctx.Done()

	// Clean shutdown - just stop log subscription, don't apply empty config
	// as it causes race conditions during rapid start/stop cycles
	m.stopLogSubscription()

	mihomolog.Infoln("Mihomo core instance context cancelled.")
}

func (m *MihomoCoreManager) Stop() error {
	m.runLock.Lock()
	defer m.runLock.Unlock()
	
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return nil
	}

	// Cancel the context to signal the runCoreAsync goroutine to stop.
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil // Prevent reuse
	}

	m.stopLogSubscription()

	m.isRunning = false
	mihomolog.Infoln("Mihomo core instance stop requested.")
	return nil
}

func (m *MihomoCoreManager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isRunning
}

func (m *MihomoCoreManager) TestConfig(configPath string) error {

	if err := m.setupEnvironment(); err != nil {
		return fmt.Errorf("failed to setup environment: %w", err)
	}

	configBytes, err := m.prepareConfigBytes(configPath)
	if err != nil {
		return fmt.Errorf("failed to prepare config: %w", err)
	}

	if _, err := executor.ParseWithBytes(configBytes); err != nil {
		return fmt.Errorf("invalid Mihomo configuration: %w", err)
	}

	mihomolog.Infoln("Mihomo configuration validation passed: %s", configPath)
	return nil
}

func (m *MihomoCoreManager) startLogSubscription() {
	m.stopLogSubscription()

	mihomolog.Infoln("Attempting to start log subscription with path: '%s'", m.logFilePath)

	if m.logFilePath == "" {
		mihomolog.Warnln("No log file path available for manual log subscription")
		return
	}

	m.logSubscriber = mihomolog.Subscribe()
	mihomolog.Infoln("Started log subscription for file: %s", m.logFilePath)

	go func() {
		logFile, err := os.OpenFile(m.logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			mihomolog.Errorln("Failed to open log file for writing: %v", err)
			return
		}
		defer logFile.Close()

		logFile.WriteString(fmt.Sprintf("[%s] Mihomo core log subscription started\n", time.Now().Format("2006-01-02 15:04:05")))

		for logData := range m.logSubscriber {
			// Log ALL messages regardless of level to ensure we don't miss anything
			logEntry := fmt.Sprintf("[%s] [%s] %s\n",
				time.Now().Format("2006-01-02 15:04:05"),
				logData.LogLevel.String(),
				logData.Payload)

			if _, err := logFile.WriteString(logEntry); err != nil {
				mihomolog.Errorln("Failed to write log entry: %v", err)
			} else {
				logFile.Sync()
			}
		}
	}()
}

func (m *MihomoCoreManager) stopLogSubscription() {
	if m.logSubscriber != nil {
		mihomolog.UnSubscribe(m.logSubscriber)
		m.logSubscriber = nil
		mihomolog.Infoln("Stopped log subscription")
	}
}

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

func (m *MihomoCoreManager) UpdateConfig(configPath string) error {
	if !m.isRunning {
		return fmt.Errorf("mihomo core is not running")
	}

	mihomolog.Infoln("Restarting Mihomo core with new configuration...")

	if err := m.Stop(); err != nil {
		return fmt.Errorf("failed to stop core: %w", err)
	}

	time.Sleep(200 * time.Millisecond)

	if err := m.RunConfig(configPath); err != nil {
		return fmt.Errorf("failed to start with new config: %w", err)
	}

	mihomolog.Infoln("Mihomo configuration updated successfully: %s", configPath)
	return nil
}
