package usb

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/gousb"
)

const (
	VendorId  = 0x04b4
	ProductId = 0xb71d
)

func FindDevice(serial string) (*Device, error) {
	const (
		maxRetries     = 10
		retryDelay     = 200 * time.Millisecond
		timeoutContext = 5 * time.Second
	)

	ctx, cancel := context.WithTimeout(context.Background(), timeoutContext)
	defer cancel()

	vendorID := gousb.ID(VendorId)
	productID := gousb.ID(ProductId)

	// First attempt: try immediately, subsequent attempts: wait for ticker
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt == 1 {
			// First attempt without delay
			device, err := findMatchingDevice(serial, vendorID, productID)
			if err != nil {
				slog.Warn("Failed to find device", "error", err, "attempt", attempt)
			}
			if device != nil {
				return device, nil
			}
			continue
		}

		// Subsequent attempts with delay
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("device search timed out after %v", timeoutContext)
		case <-time.After(retryDelay):
			device, err := findMatchingDevice(serial, vendorID, productID)
			if err != nil {
				slog.Warn("Failed to find device", "error", err, "attempt", attempt)
			}
			if device != nil {
				return device, nil
			}
		}
	}

	return nil, fmt.Errorf("no devices found matching VID %04X, PID %04X, and serial %s after %d attempts",
		vendorID, productID, serial, maxRetries)
}

// findMatchingDevice encapsulates the device discovery and matching logic
func findMatchingDevice(serial string, vendorID, productID gousb.ID) (*Device, error) {
	ctx := gousb.NewContext()

	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return desc.Vendor == vendorID && desc.Product == productID
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open devices: %w", err)
	}

	// Ensure all devices are closed except the one we return
	defer func() {
		for _, dev := range devs {
			if dev != nil {
				dev.Close()
			}
		}
	}()

	if len(devs) == 0 {
		return nil, nil // No devices found, but no error
	}

	// Find device with matching serial number
	for _, dev := range devs {
		if dev == nil {
			continue
		}

		deviceSerial, err := dev.SerialNumber()
		if err != nil {
			slog.Warn("Failed to get device serial number", "error", err)
			continue
		}

		if deviceSerial == serial {
			// Create a copy of the device pointer to avoid closing it in defer
			matchedDev := dev

			// Mark this device as nil in the slice so it won't be closed by defer
			for i, d := range devs {
				if d == dev {
					devs[i] = nil
					break
				}
			}

			return NewDevice(matchedDev, ctx)
		}
	}

	return nil, nil // No matching device found
}
