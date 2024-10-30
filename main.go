package main

import (
	"bootloader-usb/cyacdParse"
	"bootloader-usb/cybootloader_protocol"
	"bootloader-usb/uart"
	"bootloader-usb/usb"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"strings"
	"time"
)

const (
	ModeSerial = "serial"
	ModeUSB    = "usb"

	PacketSize = 64
)

var (
	peripheral  io.ReadWriteCloser
	readBuf     []byte
	globalError bool
)

func main() {
	startTime := time.Now()

	defer func(s time.Time) {
		elapsedTime := time.Since(s)
		log.Infof("Process time: %s", elapsedTime.String())
	}(startTime)

	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	})

	defer func() {
		if globalError {
			log.Fatalln("[ERROR] There was an error during the programming of the device")
		}
	}()

	filePath := flag.String("path", "", "Path for the .cyacd file")
	serial := flag.String("serial", "", "Serial for the device. Only required for USB communication")
	port := flag.String("port", "", "Port for the communication. Example: /dev/ttyACM0. Only required for serial communication")
	mode := flag.String("mode", "", "Mode of communication: usb or serial")
	key := flag.String("key", "", "Bootloader key")
	restart := flag.Bool("restart", false, "Restart the device after programming")

	flag.Parse()

	validateParams(*mode, *filePath, *port, *key, *serial)

	// parse the key
	bootloaderKeyHex, err := hex.DecodeString(*key)
	checkError(err, "Error parsing key")

	// parse the file
	f, err := cyacdParse.NewCyacd(*filePath)
	checkError(err, "Error parsing file")

	// initialize the communication method
	switch strings.ToLower(*mode) {
	case ModeSerial:
		devSerial, err := uart.NewDevice(*port)
		checkError(err, "Error creating device")
		peripheral = io.ReadWriteCloser(devSerial)
		break
	case ModeUSB:
		devUSB, err := usb.FindDevice(*serial)
		checkError(err, "Error finding device")

		if devUSB == nil {
			log.Fatalln("Device not found")
		} else {
			devUSB.Init()
			peripheral = io.ReadWriteCloser(devUSB)
		}
		break
	}

	if *restart {
		frame := cybootloader_protocol.CreateExitBootloaderCmd()
		writePeripheral(frame)
		return
	}

	frame, err := cybootloader_protocol.CreateEnterBootloaderCmd(bootloaderKeyHex)
	checkError(err, "Error creating frame")

	log.Println("Enter bootloader.")

	transactionPeripheral(frame)
	val, err := cybootloader_protocol.ParseEnterBootloaderCmdResult(readBuf)
	checkError(err, "Error parsing frame")

	if val != nil {
		if f.SiliconID() != val["siliconID"] || f.SiliconRev() != val["siliconRev"] {
			checkError(errors.New("[ERROR] The expected device does not match the detected device"), "Error")
		} else {
			var start, end uint16

			err, start, end = cybootloader_protocol.GetFlashSize(peripheral)
			if err != nil {
				checkError(errors.New("[ERROR] Error reading Flash size"), "Error")
			}
			rows := f.ParseRowData()
			totalRows := len(rows)
			for i, r := range rows {
				progress := float64(i+1) / float64(totalRows)
				printProgressBar(progress)
				//log.Printf("Row %d", i)
				if r.RowNum() < start || r.RowNum() > end {
					checkError(errors.New("[ERROR] The row number is out of range"), "Error")
				}
				result := true
				offset := uint16(0)
				for result && (r.Size()-offset+7) > PacketSize {
					subBufSize := uint16(PacketSize - 7)

					frame = cybootloader_protocol.CreateSendDataCmd(r.Data()[offset : offset+subBufSize])
					transactionPeripheral(frame)

					result = cybootloader_protocol.ParseSendDataCmdResult(readBuf)
					offset += subBufSize
				}

				if result {
					subBufSize := r.Size() - offset

					frame = cybootloader_protocol.CreateProgramRowCmd(r.Data()[offset:offset+subBufSize], r.ArrayID(), r.RowNum())
					transactionPeripheral(frame)

					if cybootloader_protocol.ParseProgramRowCmdResult(readBuf) {
						//log.Info("Row flashed")
						checksum := r.Checksum() + r.ArrayID() + byte(r.RowNum()>>8) + byte(r.RowNum()) + byte(r.Size()) + byte(r.Size()>>8)

						frame = cybootloader_protocol.CreateGetRowChecksumCmd(r.ArrayID(), r.RowNum())
						transactionPeripheral(frame)

						checksumUSB, err := cybootloader_protocol.ParseGetRowChecksumCmdResult(readBuf)
						checkError(err, "Error parsing frame")

						if checksum != checksumUSB {
							checkError(errors.New("[ERROR] The checksum does not match the expected value"), "Error")
						}

						//log.Info("Row Checksum passed")
					}
				} else {
					checkError(errors.New("[ERROR] There was an error during the programming of the device"), "Error")
				}
			}

			log.Println("")

			frame = cybootloader_protocol.CreateVerifyAppChecksumCmd()
			transactionPeripheral(frame)
			checksumApp, _ := cybootloader_protocol.ParseVerifyAppChecksumCmdResult(readBuf)

			log.Printf("Checksum app: %x", checksumApp)

			if checksumApp != 0 {
				log.Info("[SUCCESS] Device was successfully programmed")
			}

		}
	}

	log.Info("Exit bootloader. Auto reset")
	writePeripheral(cybootloader_protocol.CreateExitBootloaderCmd())

	err = peripheral.Close()
	checkError(err, "Error closing peripheral")
}

func validateParams(mode, filePath, port, key, serial string) {
	if filePath == "" {
		log.Fatalln("File path is required.")
		flag.PrintDefaults()
		return
	}

	if key == "" {
		log.Fatalln("Bootloader key is required.")
		flag.PrintDefaults()
		return
	}

	if mode == "" {
		log.Fatalln("Mode is required.")
		flag.PrintDefaults()
		return
	} else if mode != ModeSerial && mode != ModeUSB {
		log.Fatalln("Mode must be serial or usb.")
		flag.PrintDefaults()
		return
	} else {
		if mode == ModeSerial && port == "" {
			log.Fatalln("Port is required for serial mode.")
			flag.PrintDefaults()
			return
		}
		if mode == ModeUSB && serial == "" {
			log.Fatalln("Serial is required for usb mode.")
			flag.PrintDefaults()
			return
		}
	}
}

func checkError(err error, message string) {
	if err != nil {
		globalError = true
		log.WithError(err).Fatalln(message)
	}
}

func readPeripheral() {
	readBuf = make([]byte, PacketSize)
	_, err := peripheral.Read(readBuf)
	//log.Printf("Read %d bytes", n)
	checkError(err, "Error reading frame")
}

func writePeripheral(frame []byte) {
	_, err := peripheral.Write(frame)
	//log.Printf("Write %d bytes", n)
	checkError(err, "Error writing frame")

	return
}

func transactionPeripheral(frame []byte) {
	// send frame
	writePeripheral(frame)
	time.Sleep(time.Millisecond * 25)
	// read response
	readPeripheral()
}

const ProgressBarWidth = 50

func printProgressBar(progress float64) {
	// Calculate the position of the progress
	pos := int(ProgressBarWidth * progress)

	// Print the progress bar
	fmt.Printf("\r[")
	for i := 0; i < ProgressBarWidth; i++ {
		if i < pos {
			fmt.Print("=")
		} else if i == pos {
			fmt.Print(">")
		} else {
			fmt.Print(" ")
		}
	}
	// Print percentage complete
	fmt.Printf("] %3.0f%%", progress*100)
}
