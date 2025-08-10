package libunifiedcore

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type CoreType int

const (
	CoreTypeV2Ray CoreType = iota
	CoreTypeXray
	CoreTypeMihomo
	CoreTypeClash = CoreTypeMihomo
)

func (ct CoreType) String() string {
	switch ct {
	case CoreTypeV2Ray:
		return "v2ray"
	case CoreTypeXray:
		return "xray"
	case CoreTypeMihomo:
		return "mihomo"
	default:
		return "unknown"
	}
}

func (ct CoreType) DisplayName() string {
	switch ct {
	case CoreTypeV2Ray:
		return "V2Ray"
	case CoreTypeXray:
		return "Xray"
	case CoreTypeMihomo:
		return "Mihomo"
	default:
		return "Unknown"
	}
}

// IsValid checks if the CoreType is valid
func (ct CoreType) IsValid() bool {
	return ct >= CoreTypeV2Ray && ct <= CoreTypeMihomo
}

// ParseCoreType parses a string and returns the corresponding CoreType
func ParseCoreType(coreTypeStr string) (CoreType, error) {
	switch strings.ToLower(strings.TrimSpace(coreTypeStr)) {
	case "v2ray":
		return CoreTypeV2Ray, nil
	case "xray":
		return CoreTypeXray, nil
	case "mihomo":
		return CoreTypeMihomo, nil
	case "clash", "clash-meta": // Legacy support
		return CoreTypeMihomo, nil
	default:
		return CoreType(-1), fmt.Errorf("unknown core type: %s", coreTypeStr)
	}
}

func DetectCoreTypeFromConfig(configContent string) (CoreType, error) {
	configContent = strings.TrimSpace(configContent)

	// Try to detect based on content structure
	if strings.HasPrefix(configContent, "{") {
		// JSON format - likely V2Ray/Xray
		return detectJSONCoreType(configContent)
	} else if strings.Contains(configContent, "mixed-port:") ||
		strings.Contains(configContent, "external-controller:") ||
		strings.Contains(configContent, "proxy-groups:") {
		// YAML format with Mihomo/Clash specific fields
		return CoreTypeMihomo, nil
	}

	// Try YAML parsing for Mihomo
	var yamlConfig map[string]interface{}
	if err := yaml.Unmarshal([]byte(configContent), &yamlConfig); err == nil {
		if isMihomoConfig(yamlConfig) {
			return CoreTypeMihomo, nil
		}
	}

	// Try JSON parsing for V2Ray/Xray
	var jsonConfig map[string]interface{}
	if err := json.Unmarshal([]byte(configContent), &jsonConfig); err == nil {
		if isV2RayConfig(jsonConfig) {
			return CoreTypeV2Ray, nil
		}
	}

	return CoreType(-1), fmt.Errorf("unable to detect core type from configuration")
}

// detectJSONCoreType detects core type for JSON configurations
func detectJSONCoreType(configContent string) (CoreType, error) {
	var config map[string]interface{}

	if err := json.Unmarshal([]byte(configContent), &config); err != nil {
		return CoreType(-1), fmt.Errorf("invalid JSON configuration: %w", err)
	}

	// Check for V2Ray/Xray specific fields
	if isV2RayConfig(config) {
		// Check for Xray-specific features
		if isXrayConfig(config) {
			return CoreTypeXray, nil
		}
		return CoreTypeV2Ray, nil
	}

	// Check if it's actually a Mihomo JSON config
	if isMihomoConfig(config) {
		return CoreTypeMihomo, nil
	}

	return CoreType(-1), fmt.Errorf("unrecognized JSON configuration format")
}

// isV2RayConfig checks if the configuration is for V2Ray/Xray
func isV2RayConfig(config map[string]interface{}) bool {
	// V2Ray/Xray specific fields
	v2rayFields := []string{"inbounds", "outbounds", "routing", "log", "api", "dns", "policy"}

	foundFields := 0
	for _, field := range v2rayFields {
		if _, exists := config[field]; exists {
			foundFields++
		}
	}

	// If we find at least 2 V2Ray-specific fields, it's likely V2Ray/Xray
	return foundFields >= 2
}

// isXrayConfig checks if the configuration has Xray-specific features
func isXrayConfig(config map[string]interface{}) bool {
	// Check for Xray-specific features in outbounds
	if outbounds, ok := config["outbounds"].([]interface{}); ok {
		for _, outbound := range outbounds {
			if outboundMap, ok := outbound.(map[string]interface{}); ok {
				if protocol, ok := outboundMap["protocol"].(string); ok {
					// Xray-specific protocols
					xrayProtocols := []string{"vless", "xtls", "reality"}
					for _, xrayProto := range xrayProtocols {
						if strings.Contains(strings.ToLower(protocol), xrayProto) {
							return true
						}
					}
				}

				// Check for XTLS in settings
				if settings, ok := outboundMap["settings"].(map[string]interface{}); ok {
					if _, hasXTLS := settings["xtlsSettings"]; hasXTLS {
						return true
					}
				}
			}
		}
	}

	return false
}

// isMihomoConfig checks if the configuration is for Mihomo
func isMihomoConfig(config map[string]interface{}) bool {
	// Mihomo/Clash specific fields
	mihomoFields := []string{
		"mixed-port", "port", "socks-port", "redir-port", "tproxy-port",
		"external-controller", "secret", "bind-address",
		"proxy-groups", "proxies", "rules", "rule-providers",
		"mode", "log-level", "ipv6", "allow-lan",
	}

	foundFields := 0
	for _, field := range mihomoFields {
		if _, exists := config[field]; exists {
			foundFields++
		}
	}

	// If we find at least 3 Mihomo-specific fields, it's likely Mihomo
	return foundFields >= 3
}

// GetDefaultPorts returns the default ports for each core type
func (ct CoreType) GetDefaultPorts() (socksPort, httpPort, apiPort int) {
	switch ct {
	case CoreTypeV2Ray, CoreTypeXray:
		return 15491, 15492, 15490 // SOCKS, HTTP, API
	case CoreTypeMihomo:
		return 15491, 15492, 15490 // Mixed port, HTTP, External controller
	default:
		return 15491, 15492, 15490 // Default fallback
	}
}

func (ct CoreType) GetConfigFormat() string {
	switch ct {
	case CoreTypeV2Ray, CoreTypeXray:
		return "json"
	case CoreTypeMihomo:
		return "yaml" // Mihomo supports both YAML and JSON, but YAML is preferred
	default:
		return "json"
	}
}

// SupportsFormat checks if the core type supports the given configuration format
func (ct CoreType) SupportsFormat(format string) bool {
	format = strings.ToLower(strings.TrimSpace(format))

	switch ct {
	case CoreTypeV2Ray, CoreTypeXray:
		return format == "json"
	case CoreTypeMihomo:
		return format == "yaml" || format == "json"
	default:
		return false
	}
}
