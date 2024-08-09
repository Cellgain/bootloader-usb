package usb

import (
	"github.com/google/gousb"
	log "github.com/sirupsen/logrus"
	"io"
)

type Device struct {
	dev   *gousb.Device
	cfg   *gousb.Config
	intf  *gousb.Interface
	done  func()
	epIn  *gousb.InEndpoint
	epOut *gousb.OutEndpoint
}

func NewDevice(dev *gousb.Device) (*Device, error) {
	return &Device{
		dev: dev,
	}, nil
}

func (d *Device) Init() {
	var err error

	if e := d.dev.SetAutoDetach(true); e != nil {
		log.Fatalln(e)
	}

	// Switch the configuration to #1.
	d.cfg, err = d.dev.Config(1)
	if err != nil {
		log.Fatalf("%s.Config(1): %v", d.dev, err)
	}

	d.intf, d.done, err = d.dev.DefaultInterface()
	if err != nil {
		log.Fatalf("%s.Interface(0, 0): %v", d.cfg, err)
	}

	// In this interface open endpoint #2 for reading.
	d.epIn, err = d.intf.InEndpoint(2)
	if err != nil {
		log.Fatalf("%s.InEndpoint(2): %v", d.intf, err)
	}

	// And in the same interface open endpoint #1 for writing.
	d.epOut, err = d.intf.OutEndpoint(1)
	if err != nil {
		log.Fatalf("%s.OutEndpoint(1): %v", d.intf, err)
	}

}

func (d *Device) Close() error {
	if e := d.cfg.Close(); e != nil {
		return e
	}
	d.done()

	return nil
}

func (d *Device) Write(b []byte) (int, error) {
	writeBytes, err := d.epOut.Write(b)
	if err != nil {
		return writeBytes, err
	}

	return writeBytes, nil
}

func (d *Device) Read(buf []byte) (int, error) {
	readBytes, err := d.epIn.Read(buf)
	if err != nil {
		return readBytes, err
	}

	if readBytes == 0 {
		return readBytes, nil
	}

	return readBytes, nil
}

func (d *Device) CheckDev() bool {
	if d.dev == nil {
		return false
	}

	return true
}

func (d *Device) GetReadWriteCloser() io.ReadWriteCloser {
	return io.ReadWriteCloser(d)
}
