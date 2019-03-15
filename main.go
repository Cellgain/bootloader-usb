package main

import (
	"cellgain.ddns.net/cellgain-public/bootloader-usb/cyacdParse"
	"cellgain.ddns.net/cellgain-public/bootloader-usb/cybootloader_protocol"
	"cellgain.ddns.net/cellgain-public/bootloader-usb/usb"
	"flag"
	log "github.com/Sirupsen/logrus"
	"github.com/google/gousb"
)

func main() {
	log.SetLevel(log.DebugLevel)
	filePath := flag.String("path", "", "Path for the .cyacd file")

	flag.Parse()

	if *filePath == "" {
		log.Fatal("File for .cyacd is required.")
	}

	f := cyacdParse.NewCyacd(*filePath)

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
		log.Fatalln(err)
	}

	log.Println("Enter bootloader.")
	devUSB.Write(frame)

	val, e := cybootloader_protocol.ParseEnterBootloaderCmdResult(devUSB.Read())
	if e != nil {
		log.Debug(e)
	}
	log.Printf("Enter bootloader result: %#v", val)
	if f.SiliconID() != val["siliconID"] || f.SiliconRev() != val["siliconRev"]  {
		log.Debug("[ERROR] The expected device does not match the detected device")
	}

	//log.Info("Start cleaning flash")
	//cybootloader_protocol.CleanFlash(devUSB, 0x00)
	//log.Info("Finish cleaning flash")

	for i,r := range f.ParseRowData(){
		log.Info("-------------------------------------------")
		log.Printf("Row %d",i)
		if cybootloader_protocol.ValidateRow(devUSB, r){
			result := true
			offset := uint16(0)
			for result && (r.Size() - offset + 7) > 64 {
				subBufSize := uint16(64 - 7)
				frame := cybootloader_protocol.CreateSendDataCmd(r.Data()[offset:offset + subBufSize])
				log.Printf("%x", frame)
				devUSB.Write(frame)
				result = cybootloader_protocol.ParseSendDataCmdResult(devUSB.Read())
				offset += subBufSize
			}

			if result {
				subBufSize := r.Size() - offset
				frame := cybootloader_protocol.CreateProgramRowCmd(r.Data()[offset:offset + subBufSize], r.ArrayID(), r.RowNum())
				log.Printf("%x", frame)
				devUSB.Write(frame)
				if cybootloader_protocol.ParseProgramRowCmdResult(devUSB.Read()){
					log.Info("Row flashed")
					checksum := r.Checksum() + r.ArrayID() + byte(r.RowNum() >> 8) + byte(r.RowNum()) + byte(r.Size()) + byte(r.Size() >> 8)
					frame = cybootloader_protocol.CreateGetRowChecksumCmd(r.ArrayID(), r.RowNum())
					devUSB.Write(frame)
					checksumUSB, err := cybootloader_protocol.ParseGetRowChecksumCmdResult(devUSB.Read())
					if err != nil {
						log.Debug(e)
						break
					}

					if checksum != checksumUSB {
						log.Debug("[ERROR] The checksum does not match the expected value")
						break
					}

					log.Info("Row Checksum passed")
				}
			}else {
				break
			}

		}else{
			log.Println("[ERROR] The flash row is not valid")
			break
		}
	}

	devUSB.Write(cybootloader_protocol.CreateVerifyAppChecksumCmd())
	checksumApp, err := cybootloader_protocol.ParseVerifyAppChecksumCmdResult(devUSB.Read())

	log.Printf("Checksum app: %x",checksumApp)

	log.Println("Exit bootloader. Auto reset")
	devUSB.Write(cybootloader_protocol.CreateExitBootloaderCmd())

	devUSB.DeferFunctions()
}
