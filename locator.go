package hpdevices

import (
	"github.com/simulot/srvloc"
)

func LocalizeDevice() (d *HPDevice, err error) {
	//TODO: Use name to locate the correct device. At the moment, return the 1st responding device.
	device, err := srvloc.ProbeHPPrinter()
	if err == nil {
		d = &HPDevice{URL: "http://" + device.IPAddress + ":8080"}
		return d, err
	}
	return nil, err
}
