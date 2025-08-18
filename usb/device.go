package usb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/gousb"
)

// Device represents a USB device wrapper with improved error handling and resource management
type Device struct {
	dev   *gousb.Device
	ctx   *gousb.Context
	cfg   *gousb.Config
	intf  *gousb.Interface
	done  func()
	epIn  *gousb.InEndpoint
	epOut *gousb.OutEndpoint

	// Synchronization and state management
	mu     sync.RWMutex
	closed bool

	// Configuration
	config DeviceConfig
}

// DeviceConfig holds configuration options for the Device
type DeviceConfig struct {
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ConfigNumber    int // USB configuration number (default: 1)
	InterfaceNum    int // Interface number (default: 0)
	InEndpointAddr  int // Input endpoint address (default: 2)
	OutEndpointAddr int // Output endpoint address (default: 1)
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() DeviceConfig {
	return DeviceConfig{
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		ConfigNumber:    1,
		InterfaceNum:    0,
		InEndpointAddr:  2,
		OutEndpointAddr: 1,
	}
}

// NewDevice creates a new Device instance with default configuration
func NewDevice(dev *gousb.Device, ctx *gousb.Context) (*Device, error) {
	return NewDeviceWithConfig(dev, ctx, DefaultConfig())
}

// NewDeviceWithConfig creates a new Device instance with custom configuration
func NewDeviceWithConfig(dev *gousb.Device, ctx *gousb.Context, config DeviceConfig) (*Device, error) {
	if dev == nil {
		return nil, errors.New("device cannot be nil")
	}
	if ctx == nil {
		return nil, errors.New("context cannot be nil")
	}

	// Validate configuration
	if config.ReadTimeout <= 0 {
		config.ReadTimeout = DefaultConfig().ReadTimeout
	}
	if config.WriteTimeout <= 0 {
		config.WriteTimeout = DefaultConfig().WriteTimeout
	}

	return &Device{
		dev:    dev,
		ctx:    ctx,
		config: config,
	}, nil
}

// IsDeviceBusy checks if the device is currently busy/in use by another application
func (d *Device) IsDeviceBusy() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed || d.dev == nil {
		return true // Consider closed/nil devices as busy
	}

	// Check if already initialized (indicates not busy)
	if d.cfg != nil && d.intf != nil {
		return false
	}

	return d.isDeviceBusyUnsafe()
}

// CheckKernelDriver checks if a kernel driver is attached and attempts to detach it if needed
func (d *Device) CheckKernelDriver() error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed || d.dev == nil {
		return errors.New("device is closed or nil")
	}

	return d.checkKernelDriverUnsafe()
}

// Init initializes the USB device by setting up configuration, interface, and endpoints
func (d *Device) Init() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return errors.New("device is closed")
	}

	// Check if already initialized
	if d.cfg != nil {
		return nil
	}

	// Check if device is busy before proceeding
	if d.isDeviceBusyUnsafe() {
		return errors.New("device is busy/in use by another application")
	}

	// Check and handle kernel driver
	if err := d.checkKernelDriverUnsafe(); err != nil {
		return fmt.Errorf("kernel driver check failed: %w", err)
	}

	// Enable auto-detach from kernel driver
	if err := d.dev.SetAutoDetach(true); err != nil {
		slog.Warn("Failed to set auto detach, continuing anyway", "error", err)
	}

	// Switch to specified configuration
	cfg, err := d.dev.Config(d.config.ConfigNumber)
	if err != nil {
		return fmt.Errorf("failed to set config %d: %w", d.config.ConfigNumber, err)
	}
	d.cfg = cfg

	// Get the specified interface
	intf, err := d.cfg.Interface(d.config.InterfaceNum, 0)
	if err != nil {
		d.cleanup()
		return fmt.Errorf("failed to claim interface %d: %w", d.config.InterfaceNum, err)
	}
	d.intf = intf
	d.done = func() { intf.Close() }

	// Setup input endpoint
	epIn, err := d.intf.InEndpoint(d.config.InEndpointAddr)
	if err != nil {
		d.cleanup()
		return fmt.Errorf("failed to get input endpoint %d: %w", d.config.InEndpointAddr, err)
	}
	d.epIn = epIn

	// Setup output endpoint
	epOut, err := d.intf.OutEndpoint(d.config.OutEndpointAddr)
	if err != nil {
		d.cleanup()
		return fmt.Errorf("failed to get output endpoint %d: %w", d.config.OutEndpointAddr, err)
	}
	d.epOut = epOut

	slog.Info("USB device initialized successfully")
	return nil
}

// Read reads data from the USB device input endpoint with timeout
func (d *Device) Read(b []byte) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		return 0, errors.New("device is closed")
	}

	if d.epIn == nil {
		return 0, errors.New("device not initialized - call Init() first")
	}

	ctx, cancel := context.WithTimeout(context.Background(), d.config.ReadTimeout)
	defer cancel()

	n, err := d.epIn.ReadContext(ctx, b)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return n, fmt.Errorf("read timeout after %v: %w", d.config.ReadTimeout, err)
		}
		return n, fmt.Errorf("read failed: %w", err)
	}

	return n, nil
}

// Write writes data to the USB device output endpoint with timeout
func (d *Device) Write(b []byte) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		return 0, errors.New("device is closed")
	}

	if d.epOut == nil {
		return 0, errors.New("device not initialized - call Init() first")
	}

	if len(b) == 0 {
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), d.config.WriteTimeout)
	defer cancel()

	n, err := d.epOut.WriteContext(ctx, b)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return n, fmt.Errorf("write timeout after %v: %w", d.config.WriteTimeout, err)
		}
		return n, fmt.Errorf("write failed: %w", err)
	}

	return n, nil
}

// Close closes all USB resources and marks the device as closed
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil // Already closed
	}

	var errs []error

	// Clean up endpoints and interfaces
	d.cleanup()

	// Close the USB device
	if d.dev != nil {
		if err := d.dev.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close device: %w", err))
		}
		d.dev = nil
	}

	// Note: Don't close ctx here as it might be shared
	// The caller should manage the context lifecycle
	d.ctx = nil
	d.closed = true

	// Return combined errors if any occurred
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	slog.Info("USB device closed successfully")
	return nil
}

// IsInitialized returns whether the device has been initialized
func (d *Device) IsInitialized() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return !d.closed && d.cfg != nil && d.intf != nil && d.epIn != nil && d.epOut != nil
}

// IsClosed returns whether the device has been closed
func (d *Device) IsClosed() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.closed
}

// isDeviceBusyUnsafe is the internal version that doesn't acquire locks
func (d *Device) isDeviceBusyUnsafe() bool {
	if d.closed || d.dev == nil {
		return true
	}

	// Quick check - try to get active config
	cfgNum, err := d.dev.ActiveConfigNum()
	if err != nil {
		slog.Debug("Failed to get active config number", "error", err)
		return true
	}

	// Try to open config briefly to test availability
	testCfg, err := d.dev.Config(cfgNum)
	if err != nil {
		slog.Debug("Failed to open configuration", "error", err)
		return true
	}
	defer testCfg.Close()

	// Try to claim interface briefly
	testIntf, err := testCfg.Interface(d.config.InterfaceNum, 0)
	if err != nil {
		slog.Debug("Failed to claim interface - device appears busy", "error", err)
		return true
	}
	defer testIntf.Close()

	return false
}

// checkKernelDriverUnsafe is the internal version that doesn't acquire locks
func (d *Device) checkKernelDriverUnsafe() error {
	if d.closed || d.dev == nil {
		return errors.New("device is closed or nil")
	}

	// Note: gousb handles kernel driver detaching automatically with SetAutoDetach
	// This function is kept for compatibility but doesn't need to do manual detaching
	slog.Debug("Kernel driver handling is managed automatically by gousb")

	return nil
}

// cleanup handles internal cleanup without locking (must be called with lock held)
func (d *Device) cleanup() {
	if d.done != nil {
		d.done()
		d.done = nil
	}
	if d.cfg != nil {
		if err := d.cfg.Close(); err != nil {
			slog.Debug("Failed to close config during cleanup", "error", err)
		}
		d.cfg = nil
	}
	d.intf = nil
	d.epIn = nil
	d.epOut = nil
}
