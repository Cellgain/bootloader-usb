package cybootloader_protocol

import (
	"cellgain.ddns.net/cellgain-public/bootloader-usb/cyacdParse"
	"cellgain.ddns.net/cellgain-public/bootloader-usb/usb"
	"errors"
	log "github.com/Sirupsen/logrus"
)

const (
	/* The first byte of any boot loader command. */
	CmdStart = 0x01
	/* The last byte of any boot loader command. */
	CmdStop = 0x17

	BaseCmdSize = 0x07

	/* Command identifier for verifying the checksum value of the bootloadable project. */
	CmdVerifyChecksum = 0x31
	/* Command identifier for getting the number of flash rows in the target device. */
	CmdGetFlashSize = 0x32
	/* Command identifier for getting info about the app status. This is only supported on multi app bootloader. */
	CMD_GET_APP_STATUS		= 0x33
	/* Command identifier for reasing a row of flash data from the target device. */
	CmdEraseRow = 0x34
	/* Command identifier for making sure the bootloader host and bootloader are in sync. */
	CMD_SYNC 				= 0x35
	/* Command identifier for setting the active application. This is only supported on multi app bootloader. */
	CMD_SET_ACTIVE_APP 		= 0x36
	/* Command identifier for sending a block of data to the bootloader without doing anything with it yet. */
	CmdSendData = 0x37
	/* Command identifier for starting the boot loader.  All other commands ignored until this is sent. */
	CmdEnterBootloader = 0x38
	/* Command identifier for programming a single row of flash. */
	CmdProgramRow = 0x39
	/* Command identifier for verifying the contents of a single row of flash. */
	CmdGetRowChecksum = 0x3A
	/* Command identifier for exiting the bootloader and restarting the target program. */
	CmdExitBootloader = 0x3B


	CyretSuccess = 0x00
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
	const CommandDataSize = 6
	const CommandSize = BaseCmdSize + CommandDataSize
	var frame []byte

	if key == nil {
		frame = make([]byte, BaseCmdSize)
	} else {
		frame = make([]byte, CommandSize)
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

	frame := make([]byte, BaseCmdSize)

	frame[0] = CmdStart
	frame[1] = CmdExitBootloader
	frame[2] = 0x00
	frame[3] = 0x00
	frame[4], frame[5] = calcChecksum(frame)
	frame[6] = CmdStop

	return frame
}

func CreateGetFlashSizeCmd(arrayID byte) []byte{
	const CommandDataSize = 1
	const CommandSize = BaseCmdSize + CommandDataSize

	frame := make([]byte, CommandSize)
	frame[0] = CmdStart
	frame[1] = CmdGetFlashSize
	frame[2] = byte(CommandDataSize)
	frame[3] = byte(CommandDataSize) >> 8
	frame[4] = arrayID
	frame[5], frame[6] = calcChecksum(frame)
	frame[7] = CmdStop

	return frame
}

func ParseCreateGetFlashSizeCmdResult(r []byte) (map[string]uint16, error){
	const ResultDataSize = 4
	const sizeResult = BaseCmdSize + ResultDataSize

	frame := r[:sizeResult]

	if frame[1] != CyretSuccess {
		return nil, errors.New("[ERROR] The bootloader reported an error")
	}else if frame[0] != CmdStart || frame[2] != ResultDataSize || frame[3] != (ResultDataSize >> 8) || frame[sizeResult - 1] != CmdStop {
		return nil, errors.New("[ERROR] The data is not of the proper form")
	}else{
		var r = make(map[string]uint16)
		r["startRow"] = uint16(frame[5]) << 8 | uint16(frame[4])
		r["endRow"] = uint16(frame[7]) << 8 | uint16(frame[6])
		return r, nil
	}

}

func CreateSendDataCmd(b []byte) []byte{

	CommandSize := BaseCmdSize + len(b)

	frame := make([]byte, CommandSize)
	frame[0] = CmdStart
	frame[1] = CmdSendData
	frame[2] = byte(len(b))
	frame[3] = byte(len(b)) >> 8
	frame = append(frame[0:4],b...)
	frame = append(frame, 0,0,0)
	frame[len(frame) - 3],frame[len(frame) - 2] = calcChecksum(frame)
	frame[len(frame) - 1] = CmdStop

	return frame
}

func ParseDefaultCmdResult(r []byte) bool{
	const sizeResult = BaseCmdSize

	frame := r[:sizeResult]

	if frame[1] != CyretSuccess {
		return false
	}else if frame[0] != CmdStart || frame[2] != 0 || frame[3] != 0 || frame[6] != CmdStop {
		return false
	}

	return true
}

func ParseSendDataCmdResult(r []byte) bool{
	return ParseDefaultCmdResult(r)
}

func CreateProgramRowCmd(b []byte, arrayID byte, row uint16 ) []byte{
	const CommandDataSize = 3
	CommandSize := BaseCmdSize + CommandDataSize + len(b)

	frame := make([]byte, CommandSize)
	frame[0] = CmdStart
	frame[1] = CmdProgramRow
	frame[2] = byte(CommandDataSize + len(b))
	frame[3] = byte(CommandDataSize + len(b)) >> 8
	frame[4] = arrayID
	frame[5] = byte(row)
	frame[6] = byte(row) >> 8
	frame = append(frame[0:7],b...)
	frame = append(frame, 0,0,0)
	frame[len(frame) - 3],frame[len(frame) - 2] = calcChecksum(frame)
	frame[len(frame) - 1] = CmdStop

	return frame
}

func ParseProgramRowCmdResult(r []byte) bool{
	return ParseDefaultCmdResult(r)
}

func CreateGetRowChecksumCmd(arrayID byte, row uint16 ) []byte{
	const CommandDataSize = 3
	const CommandSize = BaseCmdSize + CommandDataSize

	frame := make([]byte, CommandSize)
	frame[0] = CmdStart
	frame[1] = CmdGetRowChecksum
	frame[2] = byte(CommandDataSize)
	frame[3] = byte(CommandDataSize) >> 8
	frame[4] = arrayID
	frame[5] = byte(row)
	frame[6] = byte(row) >> 8
	frame[7],frame[8] = calcChecksum(frame)
	frame[9] = CmdStop

	return frame
}

func ParseGetRowChecksumCmdResult(r []byte) (byte, error){
	const ResultDataSize = 1
	const sizeResult = BaseCmdSize + ResultDataSize

	frame := r[:sizeResult]

	if frame[1] != CyretSuccess {
		return 0xff, errors.New("[ERROR] The bootloader reported an error")
	}else if frame[0] != CmdStart || frame[2] != ResultDataSize || frame[3] != (ResultDataSize >> 8) || frame[sizeResult - 1] != CmdStop {
		return 0xff, errors.New("[ERROR] The data is not of the proper form")
	}else{

		return frame[4], nil
	}
}

func ParseEraseRowCmdResult(r []byte) bool{
	return ParseDefaultCmdResult(r)
}

func CreateEraseRowCmd(arrayID byte, row uint16 ) []byte{
	const CommandDataSize = 3
	const CommandSize = BaseCmdSize + CommandDataSize

	frame := make([]byte, CommandSize)
	frame[0] = CmdStart
	frame[1] = CmdEraseRow
	frame[2] = byte(CommandDataSize)
	frame[3] = byte(CommandDataSize) >> 8
	frame[4] = arrayID
	frame[5] = byte(row)
	frame[6] = byte(row) >> 8
	frame[7],frame[8] = calcChecksum(frame)
	frame[9] = CmdStop

	return frame
}


func CreateVerifyAppChecksumCmd() []byte{
	const CommandDataSize = 0
	const CommandSize = BaseCmdSize + CommandDataSize

	frame := make([]byte, CommandSize)
	frame[0] = CmdStart
	frame[1] = CmdVerifyChecksum
	frame[2] = byte(CommandDataSize)
	frame[3] = byte(CommandDataSize) >> 8
	frame[4],frame[5] = calcChecksum(frame)
	frame[6] = CmdStop

	return frame
}

func ParseVerifyAppChecksumCmdResult(r []byte) (byte, error){
	const ResultDataSize = 1
	const sizeResult = BaseCmdSize + ResultDataSize

	frame := r[:sizeResult]

	if frame[1] != CyretSuccess {
		return 0xff, errors.New("[ERROR] The bootloader reported an error")
	}else if frame[0] != CmdStart || frame[2] != ResultDataSize || frame[3] != (ResultDataSize >> 8) || frame[sizeResult - 1] != CmdStop {
		return 0xff, errors.New("[ERROR] The data is not of the proper form")
	}else{

		return frame[4], nil
	}
}

func ValidateRow(dev *usb.USBDevice, r *cyacdParse.Row) bool{
	frame := CreateGetFlashSizeCmd(r.ArrayID())
	dev.Write(frame)
	val, e := ParseCreateGetFlashSizeCmdResult(dev.Read())
	if e != nil {
		log.Debug(e)
	}

	if r.RowNum() < val["startRow"] || r.RowNum() > val["endRow"]{
		return false
	}

	return true
}

func CleanFlash(dev *usb.USBDevice, arrayID byte) bool{
	frame := CreateGetFlashSizeCmd(arrayID)
	dev.Write(frame)
	val, e := ParseCreateGetFlashSizeCmdResult(dev.Read())
	if e != nil {
		log.Debug(e)
	}

	for i:= val["startRow"]; i <= val["endRow"] ; i++ {
		frame := CreateEraseRowCmd(arrayID, i)
		dev.Write(frame)
		if !ParseEraseRowCmdResult(dev.Read()) {
			log.Printf("Error erasing row number: %d ", i)
		}
	}
	return true
}