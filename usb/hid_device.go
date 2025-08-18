package usb

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// HIDDevice represents a HID device wrapper with improved error handling and resource management
type HIDDevice struct {
	file   *os.File
	path   string
	serial string

	// Synchronization and state management
	mu     sync.RWMutex
	closed bool

	// Configuration
	config HIDDeviceConfig
}

// HIDDeviceConfig holds configuration options for the HID Device
type HIDDeviceConfig struct {
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DefaultHIDConfig returns a configuration with sensible defaults
func DefaultHIDConfig() HIDDeviceConfig {
	return HIDDeviceConfig{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
}

// NewHIDDevice creates a new HIDDevice instance with default configuration
func NewHIDDevice(path, serial string) (*HIDDevice, error) {
	return NewHIDDeviceWithConfig(path, serial, DefaultHIDConfig())
}

// NewHIDDeviceWithConfig creates a new HIDDevice instance with custom configuration
func NewHIDDeviceWithConfig(path, serial string, config HIDDeviceConfig) (*HIDDevice, error) {
	if path == "" {
		return nil, errors.New("device path cannot be empty")
	}
	if serial == "" {
		return nil, errors.New("serial cannot be empty")
	}

	// Validate configuration
	if config.ReadTimeout <= 0 {
		config.ReadTimeout = DefaultHIDConfig().ReadTimeout
	}
	if config.WriteTimeout <= 0 {
		config.WriteTimeout = DefaultHIDConfig().WriteTimeout
	}

	return &HIDDevice{
		path:   path,
		serial: serial,
		config: config,
	}, nil
}

// IsDeviceBusy checks if the device is currently busy/in use by another application
func (d *HIDDevice) IsDeviceBusy() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		return true
	}

	// Check if file is already open
	if d.file != nil {
		return false
	}

	// Try to open the device to check if it's available
	file, err := os.OpenFile(d.path, os.O_RDWR, 0600)
	if err != nil {
		return true
	}
	file.Close()

	return false
}

// Init initializes the HID device by opening the device file
func (d *HIDDevice) Init() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return errors.New("device is closed")
	}

	// Check if already initialized
	if d.file != nil {
		return nil
	}

	// Check if device exists
	if _, err := os.Stat(d.path); os.IsNotExist(err) {
		return fmt.Errorf("device path does not exist: %s", d.path)
	}

	// Open the HID device
	file, err := os.OpenFile(d.path, os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("failed to open HID device %s: %w", d.path, err)
	}

	d.file = file

	slog.Info("HID device initialized successfully", "path", d.path, "serial", d.serial)
	return nil
}

// Read reads data from the HID device with timeout
func (d *HIDDevice) Read(b []byte) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		return 0, errors.New("device is closed")
	}

	if d.file == nil {
		return 0, errors.New("device not initialized - call Init() first")
	}

	// Set read deadline
	if err := d.file.SetReadDeadline(time.Now().Add(d.config.ReadTimeout)); err != nil {
		return 0, fmt.Errorf("failed to set read deadline: %w", err)
	}

	n, err := d.file.Read(b)
	if err != nil {
		if os.IsTimeout(err) {
			return n, fmt.Errorf("read timeout after %v: %w", d.config.ReadTimeout, err)
		}
		return n, fmt.Errorf("read failed: %w", err)
	}

	return n, nil
}

// Write writes data to the HID device with timeout
func (d *HIDDevice) Write(b []byte) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		return 0, errors.New("device is closed")
	}

	if d.file == nil {
		return 0, errors.New("device not initialized - call Init() first")
	}

	if len(b) == 0 {
		return 0, nil
	}

	// Set write deadline
	if err := d.file.SetWriteDeadline(time.Now().Add(d.config.WriteTimeout)); err != nil {
		return 0, fmt.Errorf("failed to set write deadline: %w", err)
	}

	n, err := d.file.Write(b)
	if err != nil {
		if os.IsTimeout(err) {
			return n, fmt.Errorf("write timeout after %v: %w", d.config.WriteTimeout, err)
		}
		return n, fmt.Errorf("write failed: %w", err)
	}

	return n, nil
}

// Close closes the HID device file and marks the device as closed
func (d *HIDDevice) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil // Already closed
	}

	var err error
	if d.file != nil {
		err = d.file.Close()
		d.file = nil
	}

	d.closed = true

	if err != nil {
		return fmt.Errorf("failed to close HID device: %w", err)
	}

	slog.Info("HID device closed successfully", "path", d.path, "serial", d.serial)
	return nil
}

// IsInitialized returns whether the device has been initialized
func (d *HIDDevice) IsInitialized() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return !d.closed && d.file != nil
}

// IsClosed returns whether the device has been closed
func (d *HIDDevice) IsClosed() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.closed
}

// GetPath returns the device path
func (d *HIDDevice) GetPath() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.path
}

// GetSerial returns the device serial number
func (d *HIDDevice) GetSerial() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.serial
}
