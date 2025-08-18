package usb

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FindHIDDevice finds a HID device by serial number in /dev/hidraw_{serial} format
func FindHIDDevice(serial string) (*HIDDevice, error) {
	const (
		maxRetries     = 10
		retryDelay     = 200 * time.Millisecond
		timeoutContext = 5 * time.Second
	)

	ctx, cancel := context.WithTimeout(context.Background(), timeoutContext)
	defer cancel()

	// First attempt: try immediately, subsequent attempts: wait for delay
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt == 1 {
			// First attempt without delay
			device, err := findMatchingHIDDevice(serial)
			if err != nil {
				slog.Warn("Failed to find HID device", "error", err, "attempt", attempt, "serial", serial)
			}
			if device != nil {
				return device, nil
			}
			continue
		}

		// Subsequent attempts with delay
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("HID device search timed out after %v", timeoutContext)
		case <-time.After(retryDelay):
			device, err := findMatchingHIDDevice(serial)
			if err != nil {
				slog.Warn("Failed to find HID device", "error", err, "attempt", attempt, "serial", serial)
			}
			if device != nil {
				return device, nil
			}
		}
	}

	return nil, fmt.Errorf("no HID devices found matching serial %s after %d attempts", serial, maxRetries)
}

// findMatchingHIDDevice encapsulates the HID device discovery and matching logic
func findMatchingHIDDevice(serial string) (*HIDDevice, error) {
	// Expected device path format: /dev/hidraw_{serial}
	expectedPath := fmt.Sprintf("/dev/hidraw_%s", serial)

	// Check if the expected device path exists
	if _, err := os.Stat(expectedPath); err == nil {
		// Device exists, create HIDDevice instance
		return NewHIDDevice(expectedPath, serial)
	}

	// Fallback: scan /dev for hidraw devices and check names
	devices, err := scanHIDRawDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to scan HID raw devices: %w", err)
	}

	// Look for device with matching serial in name
	for _, path := range devices {
		if strings.Contains(path, serial) {
			return NewHIDDevice(path, serial)
		}
	}

	return nil, nil // No matching device found
}

// scanHIDRawDevices scans /dev directory for hidraw devices
func scanHIDRawDevices() ([]string, error) {
	var devices []string

	// Read /dev directory
	entries, err := os.ReadDir("/dev")
	if err != nil {
		return nil, fmt.Errorf("failed to read /dev directory: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		// Look for hidraw devices (both hidraw_* and hidrawN formats)
		if strings.HasPrefix(name, "hidraw") {
			fullPath := filepath.Join("/dev", name)
			devices = append(devices, fullPath)
		}
	}

	return devices, nil
}

// FindAllHIDDevices returns all available HID raw devices
func FindAllHIDDevices() ([]string, error) {
	return scanHIDRawDevices()
}

// IsHIDDeviceAvailable checks if a HID device with the given serial is available
func IsHIDDeviceAvailable(serial string) bool {
	expectedPath := fmt.Sprintf("/dev/hidraw_%s", serial)

	// Check if the expected device path exists
	if _, err := os.Stat(expectedPath); err == nil {
		// Try to open it to verify it's accessible
		file, err := os.OpenFile(expectedPath, os.O_RDWR, 0600)
		if err == nil {
			file.Close()
			return true
		}
	}

	return false
}
