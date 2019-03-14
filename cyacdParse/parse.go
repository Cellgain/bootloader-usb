package cyacdParse

import (
	"bufio"
	"encoding/hex"
	log "github.com/Sirupsen/logrus"
	"os"
)

type Cyacd struct {
	path string
	file *os.File
	siliconID uint32
	siliconRev uint32
}

func (c *Cyacd) SiliconRev() uint32 {
	return c.siliconRev
}

func (c *Cyacd) SiliconID() uint32 {
	return c.siliconID
}

func NewCyacd(path string) *Cyacd {
	r := &Cyacd{path: path}
	r.open()
	r.parseHeader()
	r.close()
	return r
}


func (c *Cyacd)open(){
	var err error
	c.file, err = os.Open(c.path) // For read access.
	if err != nil {
		log.Debug(err)
	}
}

func (c *Cyacd)parseHeader(){
	c.open()
	scanner := bufio.NewReader(c.file)
	line, _, e := scanner.ReadLine()
	if e != nil {
		log.Println(e)
	}

	decoded, _  := hex.DecodeString(string(line))

	c.siliconID = uint32(decoded[0]) << 24 | uint32(decoded[1]) << 16 | uint32(decoded[2]) << 8 | uint32(decoded[3])
	c.siliconRev = uint32(decoded[4])

	c.close()
}

func (c *Cyacd)close(){
	if e := c.file.Close(); e != nil {
		log.Debug(e)
	}
}

func (c *Cyacd)ParseRowData() []*Row{
	var r = make([]*Row,0)
	c.open()
	scanner := bufio.NewScanner(c.file)
	scanner.Scan()
	for scanner.Scan() {
		r = append(r, NewRow(scanner.Bytes()))
	}

	if err := scanner.Err(); err != nil {
		log.Debug(err)
	}
	return r
}