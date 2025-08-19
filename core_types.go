package libunifiedcore

import (
	"fmt"
	"strings"
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


