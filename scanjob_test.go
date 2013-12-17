package hpdevices

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

type NulImageManager struct {
	file io.WriteCloser
}

func (im *NulImageManager) NewImageWriter() (io.WriteCloser, error) {
	im.file, _ = ioutil.TempFile(os.TempDir(), "image")
	fmt.Println("NewImageWriter called")
	return im.file, nil
}

func (im *NulImageManager) CloseImage() error {
	fmt.Println("CloseImage called")
	im.file.Close()
	return nil
}

func _Test_ScanJob(t *testing.T) {
	d, err := LocalizeDevice()
	if err == nil {
		ni := new(NulImageManager)
		fmt.Println("HPDevice found at", d.URL)
		d.NewScanJob(ni, "Platen", 300, "Color")
	} else {
		fmt.Println("HPDevice error", err)
	}
	panic("Show stack")
}
