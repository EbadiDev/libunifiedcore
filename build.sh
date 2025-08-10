#!/bin/bash
set -e

# Unified Core Build Script
echo "=== Building Unified Core AAR ==="

# Configuration
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="$(pwd)"
OUTPUT_DIR="$PROJECT_ROOT/android/app/libs"
AAR_NAME="libunifiedcore.aar"
SOURCES_JAR="libunifiedcore-sources.jar"

# Go environment setup
export GOPROXY=https://goproxy.cn,direct
export CGO_ENABLED=1
export GO111MODULE=on

# Build configuration
ANDROID_API=21
LDFLAGS="-s -w"
VERBOSE_FLAG="-v"

echo "Build configuration:"
echo "  Project Root: $PROJECT_ROOT"
echo "  Build Dir: $BUILD_DIR"
echo "  Output Dir: $OUTPUT_DIR"
echo "  Android API: $ANDROID_API"
echo "  Go Proxy: $GOPROXY"

# Check dependencies
echo "=== Checking Dependencies ==="

# Check Go installation
if ! command -v go &> /dev/null; then
    echo "ERROR: Go is not installed or not in PATH"
    exit 1
fi

GO_VERSION=$(go version)
echo "Go version: $GO_VERSION"

# Check gomobile installation
if ! command -v gomobile &> /dev/null; then
    echo "Installing gomobile..."
    go install golang.org/x/mobile/cmd/gomobile@latest
    gomobile init
fi

GOMOBILE_VERSION=$(gomobile version 2>/dev/null || echo "gomobile installed")
echo "Gomobile: $GOMOBILE_VERSION"

# Verify go.mod exists
if [ ! -f "go.mod" ]; then
    echo "ERROR: go.mod not found in current directory"
    echo "Please run this script from the libunifiedcore directory"
    exit 1
fi

echo "go.mod found, checking module..."
go mod tidy
go mod download

# Clean previous builds
echo "=== Cleaning Previous Builds ==="
rm -f "$OUTPUT_DIR/$AAR_NAME"
rm -f "$OUTPUT_DIR/$SOURCES_JAR"
rm -f ./*.aar
rm -f ./*.jar

# Create output directory if it doesn't exist
mkdir -p "$OUTPUT_DIR"

# Build the AAR
echo "=== Building Unified Core AAR ==="
echo "Building for Android API $ANDROID_API..."

# Run gomobile bind
echo "Running: gomobile bind $VERBOSE_FLAG -ldflags='$LDFLAGS' -androidapi $ANDROID_API -o $OUTPUT_DIR/$AAR_NAME ./"

gomobile bind \
    $VERBOSE_FLAG \
    -ldflags="$LDFLAGS" \
    -androidapi $ANDROID_API \
    -o "$OUTPUT_DIR/$AAR_NAME" \
    ./

# Check if build was successful
if [ ! -f "$OUTPUT_DIR/$AAR_NAME" ]; then
    echo "ERROR: Failed to build $AAR_NAME"
    exit 1
fi

# Get file size for verification
AAR_SIZE=$(ls -lh "$OUTPUT_DIR/$AAR_NAME" | awk '{print $5}')
echo "Built $AAR_NAME successfully (Size: $AAR_SIZE)"

# Generate sources JAR
echo "=== Generating Sources JAR ==="
TEMP_SOURCES_DIR=$(mktemp -d)
cp -r *.go "$TEMP_SOURCES_DIR/" 2>/dev/null || true
cp go.mod "$TEMP_SOURCES_DIR/" 2>/dev/null || true
cp go.sum "$TEMP_SOURCES_DIR/" 2>/dev/null || true

if command -v jar &> /dev/null; then
    (cd "$TEMP_SOURCES_DIR" && jar cf "$OUTPUT_DIR/$SOURCES_JAR" *.go go.* 2>/dev/null || true)
    echo "Generated $SOURCES_JAR"
else
    echo "Warning: jar command not found, skipping sources JAR generation"
fi

# Cleanup temp directory
rm -rf "$TEMP_SOURCES_DIR"

# Verification
echo "=== Build Verification ==="
echo "Output files:"
ls -la "$OUTPUT_DIR"/$AAR_NAME "$OUTPUT_DIR"/$SOURCES_JAR 2>/dev/null || ls -la "$OUTPUT_DIR"/$AAR_NAME

# Test AAR structure (if unzip is available)
if command -v unzip &> /dev/null; then
    echo "=== AAR Structure Verification ==="
    TEMP_EXTRACT_DIR=$(mktemp -d)
    unzip -q "$OUTPUT_DIR/$AAR_NAME" -d "$TEMP_EXTRACT_DIR"
    echo "AAR contents:"
    find "$TEMP_EXTRACT_DIR" -type f | sort

    # Check for required files
    REQUIRED_FILES=("classes.jar" "AndroidManifest.xml")
    for file in "${REQUIRED_FILES[@]}"; do
        if [ -f "$TEMP_EXTRACT_DIR/$file" ]; then
            echo "✓ $file found"
        else
            echo "✗ $file missing"
        fi
    done

    # Check for native libraries
    if [ -d "$TEMP_EXTRACT_DIR/jni" ]; then
        echo "Native libraries:"
        find "$TEMP_EXTRACT_DIR/jni" -name "*.so" | sort
    fi

    rm -rf "$TEMP_EXTRACT_DIR"
fi

# Environment info for debugging
echo "=== Build Environment Info ==="
echo "Date: $(date)"
echo "Host: $(hostname)"
echo "User: $(whoami)"
echo "PWD: $(pwd)"
echo "Go env:"
go env GOOS GOARCH CGO_ENABLED GOPROXY GO111MODULE

# Memory usage
if command -v free &> /dev/null; then
    echo "Memory usage:"
    free -h
fi

echo ""
echo "=== Build Completed Successfully ==="
echo "Output: $OUTPUT_DIR/$AAR_NAME"
echo "Size: $AAR_SIZE"
echo ""
echo "To use in Android project:"
echo "1. Copy to android/app/libs/ (already done)"
echo "2. Add to build.gradle dependencies:"
echo "   implementation(name: 'libunifiedcore', ext: 'aar')"
echo ""
echo "Core types supported:"
echo "  - V2Ray/Xray"
echo "  - Mihomo (Clash successor)"
echo ""
echo "Default ports:"
echo "  - SOCKS: 15491 (for tun2socks)"
echo "  - API: 15490 (for dashboard)"
