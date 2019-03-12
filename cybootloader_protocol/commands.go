package cybootloader_protocol

import (
	"errors"
)

const (
	/* The first byte of any boot loader command. */
	CmdStart = 0x01
	/* The last byte of any boot loader command. */
	CmdStop = 0x17

	BaseCmdSize = 0x07

	/* Command identifier for verifying the checksum value of the bootloadable project. */
	CMD_VERIFY_CHECKSUM		= 0x31
	/* Command identifier for getting the number of flash rows in the target device. */
	CMD_GET_FLASH_SIZE		= 0x32
	/* Command identifier for getting info about the app status. This is only supported on multi app bootloader. */
	CMD_GET_APP_STATUS		= 0x33
	/* Command identifier for reasing a row of flash data from the target device. */
	CMD_ERASE_ROW			= 0x34
	/* Command identifier for making sure the bootloader host and bootloader are in sync. */
	CMD_SYNC 				= 0x35
	/* Command identifier for setting the active application. This is only supported on multi app bootloader. */
	CMD_SET_ACTIVE_APP 		= 0x36
	/* Command identifier for sending a block of data to the bootloader without doing anything with it yet. */
	CMD_SEND_DATA 			= 0x37
	/* Command identifier for starting the boot loader.  All other commands ignored until this is sent. */
	CmdEnterBootloader = 0x38
	/* Command identifier for programming a single row of flash. */
	CMD_PROGRAM_ROW 		= 0x39
	/* Command identifier for verifying the contents of a single row of flash. */
	CMD_VERIFY_ROW 			= 0x3A
	/* Command identifier for exiting the bootloader and restarting the target program. */
	CmdExitBootloader = 0x3B


	CyretSuccess = 0x00
	/* File is not accessable */
	CYRET_ERR_FILE 			= 0x01
	/* Reached the end of the file */
	CYRET_ERR_EOF 			= 0x02
	/* The amount of data available is outside the expected range */
	CYRET_ERR_LENGTH 		= 0x03
	/* The data is not of the proper form */
	CYRET_ERR_DATA 			= 0x04
	/* The command is not recognized */
	CYRET_ERR_CMD 			= 0x05
	/* The expected device does not match the detected device */
	CYRET_ERR_DEVICE 		= 0x06
	/* The bootloader version detected is not supported */
	CYRET_ERR_VERSION 		= 0x07
	/* The checksum does not match the expected value */
	CYRET_ERR_CHECKSUM 		= 0x08
	/* The flash array is not valid */
	CYRET_ERR_ARRAY 		= 0x09
	/* The flash row is not valid */
	CYRET_ERR_ROW 			= 0x0A
	/* The bootloader is not ready to process data */
	CYRET_ERR_BTLDR 		= 0x0B
	/* The application is currently marked as active */
	CYRET_ERR_ACTIVE 		= 0x0C
	/* An unknown error occured */
	CYRET_ERR_UNK 			= 0x0F
		/* The operation was aborted */
	CYRET_ABORT 			= 0xFF
)

func calcChecksum(frame []byte) (uint8, uint8) {
	var sum uint = 0

	for _, f := range frame[:len(frame)-3] {
		sum += uint(f)
	}

	result := 1 + (0xffff ^ sum)

	return uint8(result & 0xff), uint8(result >> 8)
}

func CreateEnterBootloaderCmd(key []byte) ([]byte, error) {
	var frame []byte
	if key == nil {
		frame = make([]byte, BaseCmdSize)
	} else {
		frame = make([]byte, BaseCmdSize + 6)
	}

	frame[0] = CmdStart
	frame[1] = CmdEnterBootloader

	if key != nil {
		frame[2] = 0x06
	}else{
		frame[2] = 0x00
	}

	frame[3] = 0x00

	if key != nil {
		if len(key) != 6 {
			return nil, errors.New("invalid size of key. Must be 6 bytes")
		}
		frame = append(frame[0:4],key...)
	}

	frame = append(frame, 0,0,0)
	frame[len(frame) - 3],frame[len(frame) - 2] = calcChecksum(frame)
	frame[len(frame) - 1] = CmdStop

	return frame, nil
}

func ParseEnterBootloaderCmdResult(r []byte) (map[string]uint32, error){
	const ResultDataSize = 8
	const sizeResult = BaseCmdSize + ResultDataSize

	frame := r[:sizeResult]

	if frame[1] != CyretSuccess {
		return nil, errors.New("[ERROR] The bootloader reported an error")
	}else if frame[0] != CmdStart || frame[2] != ResultDataSize || frame[3] != (ResultDataSize >> 8) || frame[sizeResult - 1] != CmdStop {
		return nil, errors.New("[ERROR] The data is not of the proper form")
	}else{
		var r = make(map[string]uint32)
		r["siliconID"] = uint32(frame[7]) << 24 | uint32(frame[6]) << 16 | uint32(frame[5]) << 8 | uint32(frame[4])
		r["siliconRev"] = uint32(frame[8])
		r["bootloaderVersion"] = uint32(frame[11]) << 16 | uint32(frame[10]) << 8 | uint32(frame[9])
		return r, nil
	}
}

func CreateExitBootloaderCmd() []byte {
	var frame []byte
	frame = make([]byte, BaseCmdSize)

	frame[0] = CmdStart
	frame[1] = CmdExitBootloader
	frame[2] = 0x00
	frame[3] = 0x00
	frame[4], frame[5] = calcChecksum(frame)
	frame[6] = CmdStop

	return frame
}