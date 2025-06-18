package usb

import (
	"errors"
	"fmt"
	"github.com/google/gousb"
	"github.com/sirupsen/logrus"
	"time"
)

const (
	VendorId  = 0x04b4
	ProductId = 0xb71d
)

func FindDevice(serial string) (*Device, error) {
	// Iterate through available Devices, finding all that match a known VID/PID.
	var err error
	devs := make([]*gousb.Device, 0)
	i := 0
	ticker := time.NewTicker(time.Millisecond * 200)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			i++
			ctx := gousb.NewContext()
			devs, err = ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
				return desc.Vendor == gousb.ID(VendorId) && desc.Product == gousb.ID(ProductId)
			})

			ctx.Close()

			if err != nil {
				logrus.WithError(err).Error("OpenDevices()")
			}
		}

		if len(devs) > 0 || i > 10 {
			break
		}
	}

	if len(devs) == 0 {
		return nil, errors.New(fmt.Sprintf("no devices found matching VID %s and PID %s", gousb.ID(VendorId), gousb.ID(ProductId)))
	}

	for idx, _ := range devs {
		if s, err := devs[idx].SerialNumber(); s == serial {
			return NewDevice(devs[idx])
		} else if err != nil {
			return nil, err
		}
		devs[idx].Close()
	}

	return nil, errors.New(fmt.Sprintf("no devices found matching Serial %s", serial))
}
