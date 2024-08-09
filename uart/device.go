package uart

import (
	"github.com/tarm/serial"
)

type Device struct {
	dev *serial.Port
}

func NewDevice(port string) (*Device, error) {
	dev, err := serial.OpenPort(
		&serial.Config{
			Name:        port,
			Baud:        115200,
			ReadTimeout: 250,
		},
	)
	if err != nil {
		return nil, err
	}

	return &Device{dev: dev}, nil
}

func (d *Device) Close() error {
	return d.dev.Close()
}

func (d *Device) Read(buf []byte) (int, error) {
	return d.dev.Read(buf)
}

func (d *Device) Write(buf []byte) (int, error) {
	return d.dev.Write(buf)
}
