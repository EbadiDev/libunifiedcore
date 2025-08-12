package libunifiedcore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	core "github.com/xtls/xray-core/core"
	serial "github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
)

type V2RayCoreManager struct {
	mu        sync.RWMutex
	instance  *core.Instance
	cancel    context.CancelFunc
	ctx       context.Context
	isRunning bool

	socksPort  int
	apiPort    int
	configPath string
	assetPath  string
	logLevel   string
	shouldOff  chan int
}

func NewV2RayCoreManager(socksPort, apiPort int) *V2RayCoreManager {
	return &V2RayCoreManager{
		socksPort: socksPort,
		apiPort:   apiPort,
		logLevel:  "info",
		shouldOff: make(chan int, 1),
	}
}

func (v *V2RayCoreManager) SetAssetPath(assetPath string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.assetPath = assetPath
}

func (v *V2RayCoreManager) SetLogLevel(logLevel string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.logLevel = logLevel
}

func (v *V2RayCoreManager) RunConfig(configPath string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.isRunning {
		return fmt.Errorf("V2Ray core is already running")
	}

	v.configPath = configPath

	// Set environment variables
	if v.assetPath != "" {
		os.Setenv("v2ray.location.asset", v.assetPath)
		os.Setenv("xray.location.asset", v.assetPath)
	}

	// Create context for cancellation
	v.ctx, v.cancel = context.WithCancel(context.Background())

	// Start core in goroutine
	go v.runConfigSync(configPath)

	// Wait a bit to ensure startup
	time.Sleep(100 * time.Millisecond)

	v.isRunning = true
	log.Printf("V2Ray core started successfully on SOCKS port %d, API port %d", v.socksPort, v.apiPort)
	return nil
}

// runConfigSync runs the core synchronously (internal method)
func (v *V2RayCoreManager) runConfigSync(configPath string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("V2Ray core panic recovered: %v", r)
		}
		v.mu.Lock()
		v.isRunning = false
		v.mu.Unlock()
	}()

	configBytes, err := v.readAndInjectConfig(configPath)
	if err != nil {
		log.Printf("Failed to read/inject V2Ray config: %v", err)
		return
	}

	// Parse configuration
	r := bytes.NewReader(configBytes)
	config, err := serial.LoadJSONConfig(r)
	if err != nil {
		log.Printf("Failed to parse V2Ray config: %v", err)
		return
	}

	// Check if already running
	v.mu.RLock()
	if v.instance != nil {
		v.mu.RUnlock()
		log.Println("V2Ray instance already exists")
		return
	}
	v.mu.RUnlock()

	// Create new instance
	instance, err := core.New(config)
	if err != nil {
		log.Printf("Failed to create V2Ray instance: %v", err)
		return
	}

	v.mu.Lock()
	v.instance = instance
	v.mu.Unlock()

	// Start the instance
	err = instance.Start()
	if err != nil {
		log.Printf("Failed to start V2Ray instance: %v", err)
		v.mu.Lock()
		v.instance = nil
		v.mu.Unlock()
		return
	}

	log.Printf("V2Ray core started and listening on SOCKS %d, API %d", v.socksPort, v.apiPort)

	// Explicitly trigger GC to remove garbage from config loading
	runtime.GC()

	// Wait for shutdown signal
	select {
	case <-v.shouldOff:
		log.Println("V2Ray core received shutdown signal")
	case <-v.ctx.Done():
		log.Println("V2Ray core context cancelled")
	}

	// Cleanup
	v.mu.Lock()
	if v.instance != nil {
		v.instance.Close()
		v.instance = nil
	}
	v.isRunning = false
	v.mu.Unlock()

	log.Println("V2Ray core stopped")
}

func (v *V2RayCoreManager) Stop() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.isRunning {
		return nil
	}

	select {
	case v.shouldOff <- 1:
	default:
		// Channel already has a signal or is closed
	}

	// Cancel context
	if v.cancel != nil {
		v.cancel()
	}

	// Force cleanup if instance still exists
	if v.instance != nil {
		v.instance.Close()
		v.instance = nil
	}

	v.isRunning = false
	log.Println("V2Ray core stopped successfully")
	return nil
}

func (v *V2RayCoreManager) IsRunning() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.isRunning && v.instance != nil
}

func (v *V2RayCoreManager) TestConfig(configPath string) error {
	// Read and inject configuration
	configBytes, err := v.readAndInjectConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to read/inject config: %w", err)
	}

	r := bytes.NewReader(configBytes)
	_, err = serial.LoadJSONConfig(r)
	if err != nil {
		return fmt.Errorf("invalid V2Ray configuration: %w", err)
	}

	log.Printf("V2Ray configuration validation passed: %s", configPath)
	return nil
}

func (v *V2RayCoreManager) readAndInjectConfig(configPath string) ([]byte, error) {

	configBytes, err := v.readFileAsBytes(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Check if this is a wrapper config
	var config map[string]interface{}
	var wrapperConfig map[string]interface{}
	if err := json.Unmarshal(configBytes, &wrapperConfig); err == nil {
		if coreConfig, exists := wrapperConfig["coreConfig"]; exists {
			// This is a wrapper config, extract the actual config
			if coreConfigMap, ok := coreConfig.(map[string]interface{}); ok {
				// coreConfig is already a map, use it directly
				config = coreConfigMap
			} else {
				return nil, fmt.Errorf("invalid coreConfig format in wrapper")
			}
		} else {
			// Not a wrapper config, use the whole config
			config = wrapperConfig
		}
	} else {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	v.injectRequiredConfig(config)

	// Marshal back to JSON
	injectedBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal injected config: %w", err)
	}

	return injectedBytes, nil
}

func (v *V2RayCoreManager) injectRequiredConfig(config map[string]interface{}) {

	if _, exists := config["inbounds"]; !exists {
		config["inbounds"] = make([]interface{}, 0)
	}

	inbounds := config["inbounds"].([]interface{})

	// Add SOCKS inbound for tun2socks (port 15491)
	socksInbound := map[string]interface{}{
		"tag":      "tun2socks-in",
		"port":     v.socksPort,
		"listen":   "127.0.0.1",
		"protocol": "socks",
		"settings": map[string]interface{}{
			"ip":        "127.0.0.1",
			"userLevel": 0,
		},
		"streamSettings": map[string]interface{}{},
	}

	// Insert at beginning to ensure priority
	inbounds = append([]interface{}{socksInbound}, inbounds...)

	// Add API inbound (port 15490)
	apiInbound := map[string]interface{}{
		"tag":      "api-in",
		"port":     v.apiPort,
		"listen":   "127.0.0.1",
		"protocol": "dokodemo-door",
		"settings": map[string]interface{}{
			"address": "127.0.0.1",
		},
	}

	inbounds = append([]interface{}{apiInbound}, inbounds...)
	config["inbounds"] = inbounds

	// Configure API
	config["api"] = map[string]interface{}{
		"tag": "api",
		"services": []string{
			"HandlerService",
			"LoggerService",
			"StatsService",
		},
	}

	// Configure stats
	config["stats"] = map[string]interface{}{}

	// Configure policy for stats
	if policy, exists := config["policy"]; exists {
		if policyMap, ok := policy.(map[string]interface{}); ok {
			if system, exists := policyMap["system"]; exists {
				if systemMap, ok := system.(map[string]interface{}); ok {
					systemMap["statsInboundUplink"] = true
					systemMap["statsInboundDownlink"] = true
				}
			} else {
				policyMap["system"] = map[string]interface{}{
					"statsInboundUplink":   true,
					"statsInboundDownlink": true,
				}
			}
		}
	} else {
		config["policy"] = map[string]interface{}{
			"system": map[string]interface{}{
				"statsInboundUplink":   true,
				"statsInboundDownlink": true,
			},
		}
	}

	// Configure routing to route API traffic to API handler
	if routing, exists := config["routing"]; exists {
		if routingMap, ok := routing.(map[string]interface{}); ok {
			if rules, exists := routingMap["rules"]; exists {
				if rulesSlice, ok := rules.([]interface{}); ok {
					// Add API routing rule at the beginning
					apiRule := map[string]interface{}{
						"type":        "field",
						"inboundTag":  []string{"api-in"},
						"outboundTag": "api",
					}
					rulesSlice = append([]interface{}{apiRule}, rulesSlice...)
					routingMap["rules"] = rulesSlice
				}
			} else {
				routingMap["rules"] = []interface{}{
					map[string]interface{}{
						"type":        "field",
						"inboundTag":  []string{"api-in"},
						"outboundTag": "api",
					},
				}
			}
		}
	} else {
		config["routing"] = map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"type":        "field",
					"inboundTag":  []string{"api-in"},
					"outboundTag": "api",
				},
			},
		}
	}

	// Ensure outbounds array exists and has API outbound
	if _, exists := config["outbounds"]; !exists {
		config["outbounds"] = make([]interface{}, 0)
	}

	outbounds := config["outbounds"].([]interface{})

	// Add API outbound
	apiOutbound := map[string]interface{}{
		"tag":      "api",
		"protocol": "blackhole",
	}

	outbounds = append(outbounds, apiOutbound)
	config["outbounds"] = outbounds

	log.Printf("V2Ray config injected - SOCKS: %d, API: %d", v.socksPort, v.apiPort)
}

// readFileAsBytes reads a file and returns its content as bytes
func (v *V2RayCoreManager) readFileAsBytes(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// Read file into byte slice
	bs := make([]byte, stat.Size())
	_, err = bufio.NewReader(file).Read(bs)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return bs, nil
}

func (v *V2RayCoreManager) GetStats() map[string]interface{} {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return map[string]interface{}{
		"core_type":    "v2ray",
		"running":      v.isRunning,
		"socks_port":   v.socksPort,
		"api_port":     v.apiPort,
		"config_path":  v.configPath,
		"asset_path":   v.assetPath,
		"log_level":    v.logLevel,
		"has_instance": v.instance != nil,
	}
}
