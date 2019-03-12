package main

import (
	"cellgain.ddns.net/cellgain-public/bootloader-usb/cybootloader_protocol"
	"cellgain.ddns.net/cellgain-public/bootloader-usb/usb"
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
	devUSB := new(usb.USBDevice)
	devUSB.Init(devs[0])
	frame, err := cybootloader_protocol.CreateEnterBootloaderCmd([]byte{0xca,0xfe,0x00,0x00,0xca,0xfe})
	if err != nil{
		log.Println(err)
	}

	log.Println("Enter bootloader.")
	devUSB.Write(frame)

	val, e := cybootloader_protocol.ParseEnterBootloaderCmdResult(devUSB.Read())
	if e != nil {
		log.Println(e)
	}
	log.Printf("Parse enter bootloader result: %#v", val)

	log.Println("Exit bootloader. Auto reset")
	devUSB.Write(cybootloader_protocol.CreateExitBootloaderCmd())

	devUSB.DeferFunctions()
}
