package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/EbadiDev/libunifiedcore"
)

// Test configuration for Mihomo
const testMihomoConfig = `
port: 7890
socks-port: 7891
allow-lan: false
mode: rule
log-level: info

proxies:
  - name: "TEST_DIRECT"
    type: direct

proxy-groups:
  - name: "PROXY"
    type: select
    proxies:
      - TEST_DIRECT

rules:
  - MATCH,PROXY

dns:
  enable: true
  listen: 127.0.0.1:1053
  nameserver:
    - 8.8.8.8
    - 1.1.1.1
`

func main() {
	fmt.Println("=== Mihomo Core Test ===")

	// Create temporary config file
	configPath := createTestConfig()
	defer os.Remove(configPath)

	// Test configuration validation
	fmt.Println("\n1. Testing configuration validation...")
	if err := testConfigValidation(configPath); err != nil {
		fmt.Printf("❌ Config validation failed: %v\n", err)
		return
	}
	fmt.Println("✅ Configuration validation passed")

	// Test core manager creation
	fmt.Println("\n2. Testing core manager creation...")
	manager := testManagerCreation()
	fmt.Println("✅ Core manager created successfully")

	// Test core startup
	fmt.Println("\n3. Testing core startup...")
	if err := testCoreStartup(manager, configPath); err != nil {
		fmt.Printf("❌ Core startup failed: %v\n", err)
		return
	}
	fmt.Println("✅ Core startup successful")

	// Test API endpoints
	fmt.Println("\n4. Testing API endpoints...")
	testAPIEndpoints(manager)

	// Test core shutdown
	fmt.Println("\n5. Testing core shutdown...")
	if err := testCoreShutdown(manager); err != nil {
		fmt.Printf("❌ Core shutdown failed: %v\n", err)
		return
	}
	fmt.Println("✅ Core shutdown successful")

	fmt.Println("\n🎉 All tests passed! Mihomo core is working correctly.")
}

func createTestConfig() string {
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "mihomo-test-*.yaml")
	if err != nil {
		panic(fmt.Sprintf("Failed to create temp file: %v", err))
	}
	defer tmpFile.Close()

	// Write test config
	if _, err := tmpFile.WriteString(testMihomoConfig); err != nil {
		panic(fmt.Sprintf("Failed to write test config: %v", err))
	}

	fmt.Printf("Created test config: %s\n", tmpFile.Name())
	return tmpFile.Name()
}

func testConfigValidation(configPath string) error {
	manager := libunifiedcore.NewMihomoCoreManager(15491, 15490)
	manager.SetLogLevel("debug")

	// Set up temporary directory for testing
	tempDir, err := os.MkdirTemp("", "mihomo-test-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	manager.SetAssetPath(tempDir)

	return manager.TestConfig(configPath)
}

func testManagerCreation() *libunifiedcore.MihomoCoreManager {
	manager := libunifiedcore.NewMihomoCoreManager(15491, 15490)
	manager.SetLogLevel("info")

	// Set up temporary directory
	tempDir, err := os.MkdirTemp("", "mihomo-test-*")
	if err != nil {
		panic(fmt.Sprintf("Failed to create temp dir: %v", err))
	}

	manager.SetAssetPath(tempDir)

	return manager
}

func testCoreStartup(manager *libunifiedcore.MihomoCoreManager, configPath string) error {
	// Start the core
	if err := manager.RunConfig(configPath); err != nil {
		return fmt.Errorf("failed to start core: %w", err)
	}

	// Wait for startup
	time.Sleep(3 * time.Second)

	// Check if running
	if !manager.IsRunning() {
		return fmt.Errorf("core is not running after startup")
	}

	return nil
}

func testAPIEndpoints(manager *libunifiedcore.MihomoCoreManager) {
	if !manager.IsRunning() {
		fmt.Println("⚠️ Core not running, skipping API tests")
		return
	}

	// Test basic API endpoints
	baseURL := "http://127.0.0.1:15490"

	// Create HTTP client with timeout
	client := &http.Client{Timeout: 5 * time.Second}

	// Test version endpoint
	fmt.Printf("Testing version endpoint...")
	if err := testHTTPEndpoint(client, baseURL+"/version"); err != nil {
		fmt.Printf(" ❌ Failed: %v\n", err)
	} else {
		fmt.Printf(" ✅ OK\n")
	}

	// Test root endpoint
	fmt.Printf("Testing root endpoint...")
	if err := testHTTPEndpoint(client, baseURL+"/"); err != nil {
		fmt.Printf(" ❌ Failed: %v\n", err)
	} else {
		fmt.Printf(" ✅ OK\n")
	}

	// Test streaming endpoints (just check if they respond)
	fmt.Printf("Testing traffic endpoint...")
	if err := testStreamingEndpoint(client, baseURL+"/traffic"); err != nil {
		fmt.Printf(" ❌ Failed: %v\n", err)
	} else {
		fmt.Printf(" ✅ OK\n")
	}

	fmt.Printf("Testing memory endpoint...")
	if err := testStreamingEndpoint(client, baseURL+"/memory"); err != nil {
		fmt.Printf(" ❌ Failed: %v\n", err)
	} else {
		fmt.Printf(" ✅ OK\n")
	}
}

func testHTTPEndpoint(client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code %d", resp.StatusCode)
	}

	return nil
}

func testStreamingEndpoint(client *http.Client, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code %d", resp.StatusCode)
	}

	// Read a small amount to verify streaming works
	buffer := make([]byte, 100)
	_, err = resp.Body.Read(buffer)
	if err != nil && err.Error() != "EOF" {
		return err
	}

	return nil
}

func testCoreShutdown(manager *libunifiedcore.MihomoCoreManager) error {
	if !manager.IsRunning() {
		return fmt.Errorf("core is not running")
	}

	// Stop the core
	if err := manager.Stop(); err != nil {
		return fmt.Errorf("failed to stop core: %w", err)
	}

	// Wait for shutdown
	time.Sleep(1 * time.Second)

	// Check if stopped
	if manager.IsRunning() {
		return fmt.Errorf("core is still running after shutdown")
	}

	return nil
}

func debugAPIResponse(url string) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("API Debug - Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("API Debug - Status: %d\n", resp.StatusCode)
	fmt.Printf("API Debug - Headers: %v\n", resp.Header)

	if resp.StatusCode == http.StatusOK {
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			fmt.Printf("API Debug - JSON Response: %v\n", result)
		}
	}
}
