package cyacdParse

import (
	"bufio"
	"encoding/hex"
	log "github.com/sirupsen/logrus"
	"os"
)

type Cyacd struct {
	path       string
	file       *os.File
	siliconID  uint32
	siliconRev uint32
}

func (c *Cyacd) SiliconRev() uint32 {
	return c.siliconRev
}

func (c *Cyacd) SiliconID() uint32 {
	return c.siliconID
}

func NewCyacd(path string) (*Cyacd, error) {
	r := &Cyacd{
		path: path,
	}

	if err := r.parseHeader(); err != nil {
		return nil, err
	}

	return r, nil
}

func (c *Cyacd) open() error {
	var err error
	c.file, err = os.Open(c.path) // For read access.
	if err != nil {
		return err
	}
	return nil
}

func (c *Cyacd) parseHeader() error {
	if err := c.open(); err != nil {
		return err
	}
	defer c.close()

	scanner := bufio.NewReader(c.file)
	line, _, err := scanner.ReadLine()
	if err != nil {
		return err
	}

	decoded, _ := hex.DecodeString(string(line))

	c.siliconID = uint32(decoded[0])<<24 | uint32(decoded[1])<<16 | uint32(decoded[2])<<8 | uint32(decoded[3])
	c.siliconRev = uint32(decoded[4])

	return nil
}

func (c *Cyacd) close() {
	if e := c.file.Close(); e != nil {
		log.Debug(e)
	}
}

func (c *Cyacd) ParseRowData() []*Row {
	var r = make([]*Row, 0)
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
