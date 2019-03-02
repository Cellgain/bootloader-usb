package main

import (
	"fmt"
	"github.com/google/gousb"
	"log"
)

func main() {
	// Initialize a new Context.
	ctx := gousb.NewContext()
	defer ctx.Close()

	// Iterate through available Devices, finding all that match a known VID/PID.
	vid, pid := gousb.ID(0x04b4), gousb.ID(0xb71d)
	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		// this function is called for every device present.
		// Returning true means the device should be opened.
		return desc.Vendor == vid && desc.Product == pid
	})
	// All returned devices are now open and will need to be closed.
	for _, d := range devs {
		defer d.Close()
	}
	if err != nil {
		log.Fatalf("OpenDevices(): %v", err)
	}
	if len(devs) == 0 {
		log.Fatalf("no devices found matching VID %s and PID %s", vid, pid)
	}

	// Pick the first device found.
	dev := devs[0]
	dev.SetAutoDetach(true)

	// Switch the configuration to #2.
	cfg, err := dev.Config(1)
	if err != nil {
		log.Fatalf("%s.Config(2): %v", dev, err)
	}
	defer cfg.Close()

	// In the config #2, claim interface #3 with alt setting #0.
	intf, done, err := dev.DefaultInterface()
	if err != nil {
		log.Fatalf("%s.Interface(3, 0): %v", cfg, err)
	}
	defer done()

	// In this interface open endpoint #6 for reading.
	epIn, err := intf.InEndpoint(2)
	if err != nil {
		log.Fatalf("%s.InEndpoint(2): %v", intf, err)
	}

	// And in the same interface open endpoint #5 for writing.
	epOut, err := intf.OutEndpoint(1)
	if err != nil {
		log.Fatalf("%s.OutEndpoint(1): %v", intf, err)
	}

	// Buffer large enough for 10 USB packets from endpoint 6.
	buf := make([]byte, 10*epIn.Desc.MaxPacketSize)

	// writeBytes might be smaller than the buffer size if an error occurred. writeBytes might be greater than zero even if err is not nil.
	writeBytes, err := epOut.Write([]byte{0x01,0x3c,0x01,0x00,0x3e,0x17})
	if err != nil {
		fmt.Println("Write returned an error:", err)
	}

	log.Printf("Bytes wrote: %d\r\n", writeBytes)

	readBytes, err := epIn.Read(buf)
	if err != nil {
		fmt.Println("Read returned an error:", err)
	}
	if readBytes == 0 {
		log.Fatalf("IN endpoint 6 returned 0 bytes of data.")
	}

	fmt.Println(readBytes)

}
