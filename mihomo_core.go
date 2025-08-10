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
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return fmt.Errorf("Mihomo core is already running")
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

	time.Sleep(300 * time.Millisecond)

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

	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(configBytes, &config); err != nil {

		if err := json.Unmarshal(configBytes, &config); err != nil {
			return nil, fmt.Errorf("failed to parse config as YAML or JSON: %w", err)
		}
	}

	m.injectRequiredConfig(config)

	injectedBytes, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal injected config: %w", err)
	}

	return injectedBytes, nil
}

func (m *MihomoCoreManager) injectRequiredConfig(config map[string]interface{}) {

	config["mixed-port"] = m.socksPort
	config["bind-address"] = "127.0.0.1"

	config["external-controller"] = fmt.Sprintf("127.0.0.1:%d", m.apiPort)

	config["allow-lan"] = false
	config["log-level"] = m.logLevel

	if _, exists := config["mode"]; !exists {
		config["mode"] = "rule"
	}

	if _, exists := config["proxies"]; !exists {
		config["proxies"] = []interface{}{
			map[string]interface{}{
				"name": "DIRECT",
				"type": "direct",
			},
		}
	}

	if _, exists := config["proxy-groups"]; !exists {
		config["proxy-groups"] = []interface{}{
			map[string]interface{}{
				"name":    "PROXY",
				"type":    "select",
				"proxies": []string{"DIRECT"},
			},
		}
	}

	if _, exists := config["rules"]; !exists {
		config["rules"] = []interface{}{
			"MATCH,PROXY",
		}
	}

	if _, exists := config["dns"]; !exists {
		config["dns"] = map[string]interface{}{
			"enable":             true,
			"listen":             "127.0.0.1:1053",
			"default-nameserver": []string{"8.8.8.8", "1.1.1.1"},
			"nameserver":         []string{"8.8.8.8", "1.1.1.1"},
		}
	}

	if _, exists := config["log-file"]; !exists {

		logDir := filepath.Join(m.assetPath, "log")
		if m.assetPath == "" && m.configDir != "" {
			logDir = filepath.Join(m.configDir, "log")
		}
		if logDir != "" {
			os.MkdirAll(logDir, 0755)
			logFile := filepath.Join(logDir, "core.log")
			config["log-file"] = logFile
			mihomolog.Infoln("Added fallback log-file: %s", logFile)
		}
	} else {
		if logFile, ok := config["log-file"].(string); ok {
			mihomolog.Infoln("Using Flutter-injected log-file: %s", logFile)

			logDir := filepath.Dir(logFile)
			if err := os.MkdirAll(logDir, 0755); err != nil {
				mihomolog.Warnln("Failed to create log directory %s: %v", logDir, err)
			}
		} else {
			mihomolog.Infoln("Using Flutter-injected log-file configuration (non-string type)")
		}
	}

	if logFile, exists := config["log-file"]; exists {
		if logPath, ok := logFile.(string); ok {
			m.logFilePath = logPath
		}
	}

	mihomolog.Infoln("Mihomo config injected - Mixed port: %d, External controller: 127.0.0.1:%d", m.socksPort, m.apiPort)
}

func (m *MihomoCoreManager) runCoreAsync(configBytes []byte) {
	defer func() {
		if r := recover(); r != nil {
			mihomolog.Errorln("Mihomo core panic recovered: %v", r)
		}
		m.mu.Lock()
		m.isRunning = false
		m.mu.Unlock()
	}()

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

	hub.ApplyConfig(parsedConfig)

	mihomolog.SetLevel(parsedConfig.General.LogLevel)
	mihomolog.Infoln("Mihomo: Log level set to: %s", parsedConfig.General.LogLevel.String())

	m.startLogSubscription()

	mihomolog.Infoln("Mihomo core started successfully via hub.ApplyConfig")

	// Wait for shutdown signal
	<-m.ctx.Done()

	m.stopLogSubscription()

	executor.Shutdown()
	mihomolog.Infoln("Mihomo core stopped")
}

func (m *MihomoCoreManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return nil
	}

	if m.cancel != nil {
		m.cancel()
	}

	time.Sleep(100 * time.Millisecond)

	m.isRunning = false
	mihomolog.Infoln("Mihomo core stopped successfully")
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
			if logData.LogLevel < mihomolog.Level() {
				continue
			}

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
		return fmt.Errorf("Mihomo core is not running")
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
