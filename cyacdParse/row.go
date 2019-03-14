package cyacdParse

import (
	"encoding/hex"
)

type Row struct {
	arrayID uint8
	rowNum uint16
	size uint16
	checksum uint8
	data []byte
}

func (r *Row) Data() []byte {
	return r.data
}

func (r *Row) Checksum() uint8 {
	return r.checksum
}

func (r *Row) RowNum() uint16 {
	return r.rowNum
}

func (r *Row) Size() uint16 {
	return r.size
}

func (r *Row) ArrayID() uint8 {
	return r.arrayID
}

func (r *Row)ProgramRow(){

}

func NewRow(r []byte) *Row {
	decoded, _  := hex.DecodeString(string(r[1:]))
	size := uint16(decoded[3]) << 8 | uint16(decoded[4])
	return &Row{
		decoded[0],
		uint16(decoded[1]) << 8 | uint16(decoded[2]),
		size,
		decoded[len(decoded) - 1],
		decoded[5:size + 5],
	}
}



