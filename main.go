package main

import (
	"cellgain.ddns.net/cellgain-public/bootloader-usb/cyacdParse"
	"cellgain.ddns.net/cellgain-public/bootloader-usb/cybootloader_protocol"
	"cellgain.ddns.net/cellgain-public/bootloader-usb/usb"
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/google/gousb"
	"os"
)

func main() {
	errorGlobal := false

	log.SetLevel(log.DebugLevel)
	filePath := flag.String("path", "", "Path for the .cyacd file")
	serial := flag.String("serial", "", "Serial of the device")

	flag.Parse()

	if *filePath == "" {
		log.Error("Path for .cyacd is required.")
		_, _ = fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		return
	}

	if *serial == "" {
		log.Error("Device serial is required.")
		_, _ = fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		return
	}

	f := cyacdParse.NewCyacd(*filePath)

	// Initialize a new Context.
	ctx := gousb.NewContext()
	defer ctx.Close()
	// Iterate through available Devices, finding all that match a known VID/PID.
	vid, pid := gousb.ID(0x04b4), gousb.ID(0xb71d)
	i:= 0
	devs := make([]*gousb.Device,0)
	var err error
	for{
		devs, err = ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
			// this function is called for every device present.
			// Returning true means the device should be opened.
			return desc.Vendor == vid && desc.Product == pid
		})

		if len(devs) > 0 || i > 10 {
			break
		}
		i++
	}

	// All returned devices are now open and will need to be closed.
	if err != nil {
		log.Fatalf("OpenDevices(): %v", err)
	}

	for _, d := range devs {
		defer d.Close()
	}

	if len(devs) == 0 {
		log.Fatalf("no devices found matching VID %s and PID %s", vid, pid)
	}


	devUSB := new(usb.USBDevice)
	for _, d := range devs {
		if s, err := d.SerialNumber(); err == nil{
			if s == *serial {
				devUSB.Init(d)
			}
		}else{
			log.Fatalln(err)
		}
	}

	if !devUSB.CheckDev(){
		log.Fatalf("no devices found matching Serial %s", *serial)
	}

	frame, err := cybootloader_protocol.CreateEnterBootloaderCmd([]byte{0xca,0xfe,0x00,0x00,0xca,0xfe})
	if err != nil{
		log.Fatalln(err)
	}

	log.Println("Enter bootloader.")
	devUSB.Write(frame)

	val, e := cybootloader_protocol.ParseEnterBootloaderCmdResult(devUSB.Read())
	if e != nil {
		log.Error(e)
	}

	if f.SiliconID() != val["siliconID"] || f.SiliconRev() != val["siliconRev"]  {
		log.Error("[ERROR] The expected device does not match the detected device")
		errorGlobal = true
	}else{
		for i,r := range f.ParseRowData(){
			log.Info("-------------------------------------------")
			log.Printf("Row %d",i)
			if cybootloader_protocol.ValidateRow(devUSB, r){
				result := true
				offset := uint16(0)
				for result && (r.Size() - offset + 7) > 64 {
					subBufSize := uint16(64 - 7)
					frame := cybootloader_protocol.CreateSendDataCmd(r.Data()[offset:offset + subBufSize])
					devUSB.Write(frame)
					result = cybootloader_protocol.ParseSendDataCmdResult(devUSB.Read())
					offset += subBufSize
				}

				if result {
					subBufSize := r.Size() - offset
					frame := cybootloader_protocol.CreateProgramRowCmd(r.Data()[offset:offset + subBufSize], r.ArrayID(), r.RowNum())

					devUSB.Write(frame)
					if cybootloader_protocol.ParseProgramRowCmdResult(devUSB.Read()){
						log.Info("Row flashed")
						checksum := r.Checksum() + r.ArrayID() + byte(r.RowNum() >> 8) + byte(r.RowNum()) + byte(r.Size()) + byte(r.Size() >> 8)
						frame = cybootloader_protocol.CreateGetRowChecksumCmd(r.ArrayID(), r.RowNum())
						devUSB.Write(frame)
						checksumUSB, err := cybootloader_protocol.ParseGetRowChecksumCmdResult(devUSB.Read())
						if err != nil {
							log.Error(e)
							errorGlobal = true
							break
						}

						if checksum != checksumUSB {
							log.Error("[ERROR] The checksum does not match the expected value")
							errorGlobal = true
							break
						}

						log.Info("Row Checksum passed")
					}
				}else {
					break
				}

			}else{
				log.Error("[ERROR] The flash row is not valid")
				errorGlobal = true
				break
			}
		}

		devUSB.Write(cybootloader_protocol.CreateVerifyAppChecksumCmd())
		checksumApp, _ := cybootloader_protocol.ParseVerifyAppChecksumCmdResult(devUSB.Read())

		log.Printf("Checksum app: %x",checksumApp)

		if checksumApp != 0 && errorGlobal == false {
			log.Info("[SUCCESS] Device was successfully programmed")
		}

	}

	log.Info("Exit bootloader. Auto reset")
	devUSB.Write(cybootloader_protocol.CreateExitBootloaderCmd())

	devUSB.DeferFunctions()

	if errorGlobal {
		log.Fatalln("[ERROR] There was an error during the programming of the device")
	}
}
