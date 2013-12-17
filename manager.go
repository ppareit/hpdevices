package hpdevices

import (
	"net/http"
	"strings"
)

type HPDevice struct {
	URL string
}

type HPDeviceError struct {
	Operation string
	Message   string
	Err       error
}

// Implement Error
func (e HPDeviceError) Error() string {
	if e.Err != nil {
		return e.Operation + ": " + e.Message + ", " + e.Err.Error()
	} else {
		return e.Operation + ": " + e.Message
	}
}

func NewHPDeviceError(operation, message string, err ...error) error {
	e := HPDeviceError{operation, message, nil}
	if len(err) > 0 {
		e.Err = err[0]
	}
	return e
}

func NewHPDevice(url string) (d *HPDevice, err error) {
	d = &HPDevice{url}
	err = d.IsOnLine()
	if err != nil {
		return nil, err
	}
	return d, err
}

func (d *HPDevice) IsOnLine() (err error) {
	resp, err := http.Get(d.URL + "/DevMgmt/DiscoveryTree.xml")
	if resp != nil {
		resp.Body.Close()
	}
	return err
}

// Utilities
// Extract UUID placed at the right end of the URI
// Will be used to check wich client is concerned
func getUUIDfromURI(uri string) string {
	return uri[strings.LastIndex(uri, "/")+1:]
}
