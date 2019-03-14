package usb

import (
	log "github.com/Sirupsen/logrus"
	"github.com/google/gousb"
)

type USBDevice struct{
	dev *gousb.Device
	cfg *gousb.Config
	intf *gousb.Interface
	done func()
	epIn *gousb.InEndpoint
	epOut *gousb.OutEndpoint


}

func (d *USBDevice)Init(dev *gousb.Device){
	var err error
	d.dev = dev
	if e := d.dev.SetAutoDetach(true); e != nil {
		log.Fatalln(e)
	}

	// Switch the configuration to #1.
	d.cfg, err = d.dev.Config(1)
	if err != nil {
		log.Fatalf("%s.Config(1): %v", dev, err)
	}

	d.intf, d.done, err = dev.DefaultInterface()
	if err != nil {
		log.Fatalf("%s.Interface(0, 0): %v", d.cfg, err)
	}


	// In this interface open endpoint #2 for reading.
	d.epIn, err = d.intf.InEndpoint(2)
	if err != nil {
		log.Fatalf("%s.InEndpoint(2): %v", d.intf, err)
	}

	// And in the same interface open endpoint #1 for writing.
	d.epOut, err = d.intf.OutEndpoint(1)
	if err != nil {
		log.Fatalf("%s.OutEndpoint(1): %v", d.intf, err)
	}

}

func (d *USBDevice)DeferFunctions(){
	if e := d.cfg.Close(); e != nil{
		log.Fatalln(e)
	}
	d.done()
}

func (d *USBDevice)Write(b []byte){
	_ , err := d.epOut.Write(b)
	if err != nil {
		log.Println("Write returned an error:", err)
	}
}

func (d *USBDevice)Read() []byte{
	buf := make([]byte, 64)
	readBytes, err := d.epIn.Read(buf)
	if err != nil {
		log.Println("Read returned an error:", err)
	}

	if readBytes == 0 {
		log.Println("IN endpoint 2 returned 0 bytes of data.")
	}

	return buf
}