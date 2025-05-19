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
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	ModeSerial = "serial"
	ModeUSB    = "usb"

	PacketSize = 64
	AppVersion = "1.0.0"

	// Event type constants
	EventProcessStart    = "process.start"
	EventProcessComplete = "process.complete"

	// Bootloader communication events
	EventBootloaderEnter = "bootloader.enter"
	EventBootloaderExit  = "bootloader.exit"

	// Programming events
	EventProgrammingStart    = "programming.start"
	EventProgrammingProgress = "programming.progress"
	EventProgrammingComplete = "programming.complete"

	// Verification events
	EventVerificationStart    = "verification.start"
	EventVerificationComplete = "verification.complete"

	// Error events
	EventError              = "error"
	EventCommunicationError = "error.communication"
	EventValidationError    = "error.validation"

	// Error code constants
	ErrorCodeParamValidation  = "ERR_PARAM_VALIDATION"
	ErrorCodeDeviceNotFound   = "ERR_DEVICE_NOT_FOUND"
	ErrorCodeCommunication    = "ERR_COMMUNICATION"
	ErrorCodeProgramming      = "ERR_PROGRAMMING"
	ErrorCodeVerification     = "ERR_VERIFICATION"
	ErrorCodeDeviceMismatch   = "ERR_DEVICE_MISMATCH"
	ErrorCodeOutOfRange       = "ERR_OUT_OF_RANGE"
	ErrorCodeChecksumMismatch = "ERR_CHECKSUM_MISMATCH"
)

var (
	peripheral  io.ReadWriteCloser
	readBuf     []byte
	globalError bool
	processID   string
)

func init() {
	// Generate a unique ID for this process run
	processID = fmt.Sprintf("%d", time.Now().UnixNano())

	// Initialize slog with JSON handler
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(jsonHandler)
	slog.SetDefault(logger)
}

// Helper function for standardized logging
func logEvent(level slog.Level, eventType string, message string, attrs ...any) {
	// Add standard fields to every log
	standardAttrs := []any{
		"event_type", eventType,
		"process_id", processID,
	}

	// Combine standard and custom attributes
	allAttrs := append(standardAttrs, attrs...)

	// Log with appropriate level
	switch level {
	case slog.LevelDebug:
		slog.Debug(message, allAttrs...)
	case slog.LevelInfo:
		slog.Info(message, allAttrs...)
	case slog.LevelWarn:
		slog.Warn(message, allAttrs...)
	case slog.LevelError:
		slog.Error(message, allAttrs...)
	}
}

func main() {
	startTime := time.Now()

	filePath := flag.String("path", "", "Path for the .cyacd file")
	serial := flag.String("serial", "", "Serial for the device. Only required for USB communication")
	port := flag.String("port", "", "Port for the communication. Example: /dev/ttyACM0. Only required for serial communication")
	mode := flag.String("mode", "", "Mode of communication: usb or serial")
	key := flag.String("key", "", "Bootloader key")
	restart := flag.Bool("restart", false, "Restart the device after programming")

	flag.Parse()

	// Log application start with version and configuration
	logEvent(slog.LevelInfo, EventProcessStart, "Bootloader USB process started",
		"version", AppVersion,
		"mode", *mode,
		"file", *filePath)

	defer func(s time.Time) {
		elapsedTime := time.Since(s)
		logEvent(slog.LevelInfo, EventProcessComplete, "Process completed", "duration", elapsedTime.String())
	}(startTime)

	defer func() {
		if globalError {
			logEvent(slog.LevelError, EventError, "There was an error during the programming of the device",
				"error_code", ErrorCodeProgramming)
			os.Exit(1)
		}
	}()

	validateParams(*mode, *filePath, *port, *key, *serial)

	// parse the key
	bootloaderKeyHex, err := hex.DecodeString(*key)
	checkError(err, "Error parsing key", ErrorCodeParamValidation)

	// parse the file
	f, err := cyacdParse.NewCyacd(*filePath)
	checkError(err, "Error parsing file", ErrorCodeParamValidation)

	// initialize the communication method
	switch strings.ToLower(*mode) {
	case ModeSerial:
		devSerial, err := uart.NewDevice(*port)
		checkError(err, "Error creating device", ErrorCodeDeviceNotFound)
		peripheral = io.ReadWriteCloser(devSerial)
		break
	case ModeUSB:
		devUSB, err := usb.FindDevice(*serial)
		checkError(err, "Error finding device", ErrorCodeDeviceNotFound)

		if devUSB == nil {
			logEvent(slog.LevelError, EventError, "Device not found",
				"error_code", ErrorCodeDeviceNotFound)
			os.Exit(1)
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
	checkError(err, "Error creating frame", ErrorCodeCommunication)

	logEvent(slog.LevelInfo, EventBootloaderEnter, "Enter bootloader", "phase", "initialization")

	transactionPeripheral(frame)
	val, err := cybootloader_protocol.ParseEnterBootloaderCmdResult(readBuf)
	checkError(err, "Error parsing frame", ErrorCodeCommunication)

	if val != nil {
		if f.SiliconID() != val["siliconID"] || f.SiliconRev() != val["siliconRev"] {
			checkError(errors.New("[ERROR] The expected device does not match the detected device"), "Device mismatch error", ErrorCodeDeviceMismatch)
		} else {
			var start, end uint16

			err, start, end = cybootloader_protocol.GetFlashSize(peripheral)
			if err != nil {
				checkError(errors.New("[ERROR] Error reading Flash size"), "Error reading Flash size", ErrorCodeCommunication)
			}
			rows := f.ParseRowData()
			totalRows := len(rows)
			var lastReportedProgress int = 0

			logEvent(slog.LevelInfo, EventProgrammingStart, "Starting programming process",
				"phase", "programming",
				"progress", 0,
				"total_rows", totalRows)

			for i, r := range rows {
				progress := float64(i+1) / float64(totalRows)
				currentProgressPercent := int(progress * 100)

				// Report progress at 10% intervals
				if currentProgressPercent/10 > lastReportedProgress/10 {
					lastReportedProgress = currentProgressPercent
					logEvent(slog.LevelInfo, EventProgrammingProgress, "Programming progress",
						"phase", "programming",
						"progress", currentProgressPercent,
						"current_row", i+1,
						"total_rows", totalRows)
				}

				if r.RowNum() < start || r.RowNum() > end {
					checkError(errors.New("[ERROR] The row number is out of range"), "Row number out of range error", ErrorCodeOutOfRange)
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
						checkError(err, "Error parsing frame", ErrorCodeCommunication)

						if checksum != checksumUSB {
							checkError(errors.New("[ERROR] The checksum does not match the expected value"), "Checksum mismatch error", ErrorCodeChecksumMismatch)
						}

						//log.Info("Row Checksum passed")
					}
				} else {
					checkError(errors.New("[ERROR] There was an error during the programming of the device"), "Programming error", ErrorCodeProgramming)
				}
			}

			logEvent(slog.LevelInfo, EventProgrammingComplete, "Programming completed", "phase", "verification")

			frame = cybootloader_protocol.CreateVerifyAppChecksumCmd()
			transactionPeripheral(frame)
			checksumApp, _ := cybootloader_protocol.ParseVerifyAppChecksumCmdResult(readBuf)

			logEvent(slog.LevelInfo, EventVerificationComplete, "Application checksum",
				"checksum", fmt.Sprintf("%x", checksumApp),
				"phase", "verification")

			if checksumApp != 0 {
				logEvent(slog.LevelInfo, EventProcessComplete, "Device was successfully programmed",
					"status", "success",
					"phase", "completion")
			}

		}
	}

	logEvent(slog.LevelInfo, EventBootloaderExit, "Exit bootloader. Auto reset", "phase", "completion")
	writePeripheral(cybootloader_protocol.CreateExitBootloaderCmd())

	err = peripheral.Close()
	checkError(err, "Error closing peripheral", ErrorCodeCommunication)
}

func validateParams(mode, filePath, port, key, serial string) {
	if filePath == "" {
		logEvent(slog.LevelError, EventValidationError, "File path is required.",
			"error_code", ErrorCodeParamValidation)
		flag.PrintDefaults()
		os.Exit(1)
	}

	if key == "" {
		logEvent(slog.LevelError, EventValidationError, "Bootloader key is required.",
			"error_code", ErrorCodeParamValidation)
		flag.PrintDefaults()
		os.Exit(1)
	}

	if mode == "" {
		logEvent(slog.LevelError, EventValidationError, "Mode is required.",
			"error_code", ErrorCodeParamValidation)
		flag.PrintDefaults()
		os.Exit(1)
	} else if mode != ModeSerial && mode != ModeUSB {
		logEvent(slog.LevelError, EventValidationError, "Mode must be serial or usb.",
			"error_code", ErrorCodeParamValidation)
		flag.PrintDefaults()
		os.Exit(1)
	} else {
		if mode == ModeSerial && port == "" {
			logEvent(slog.LevelError, EventValidationError, "Port is required for serial mode.",
				"error_code", ErrorCodeParamValidation)
			flag.PrintDefaults()
			os.Exit(1)
		}
		if mode == ModeUSB && serial == "" {
			logEvent(slog.LevelError, EventValidationError, "Serial is required for usb mode.",
				"error_code", ErrorCodeParamValidation)
			flag.PrintDefaults()
			os.Exit(1)
		}
	}
}

func checkError(err error, message string, errorCode ...string) {
	if err != nil {
		globalError = true
		code := ErrorCodeProgramming
		if len(errorCode) > 0 {
			code = errorCode[0]
		}
		logEvent(slog.LevelError, EventError, message,
			"error", err.Error(),
			"error_code", code)
		os.Exit(1)
	}
}

func readPeripheral() {
	readBuf = make([]byte, PacketSize)
	_, err := peripheral.Read(readBuf)
	//log.Printf("Read %d bytes", n)
	if err != nil {
		if err.Error() == "read operation timed out" || err.Error() == "timeout" {
			checkError(err, "Communication timeout: device is unresponsive or not in bootloader mode", ErrorCodeCommunication)
		} else {
			checkError(err, "Error reading frame", ErrorCodeCommunication)
		}
	}
}

func writePeripheral(frame []byte) {
	_, err := peripheral.Write(frame)
	//log.Printf("Write %d bytes", n)
	checkError(err, "Error writing frame", ErrorCodeCommunication)

	return
}

func transactionPeripheral(frame []byte) {
	// send frame
	writePeripheral(frame)
	time.Sleep(time.Millisecond * 25)
	// read response
	readPeripheral()
}
