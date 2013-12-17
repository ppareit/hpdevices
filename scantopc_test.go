package hpdevices

// +build ignore

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

type NulBatchImageManager struct {
	file io.WriteCloser
}

func (im *NulBatchImageManager) NewImageWriter() (io.WriteCloser, error) {
	im.file, _ = ioutil.TempFile(os.TempDir(), "image")
	fmt.Println("NewImageWriter called")
	return im.file, nil
}

func (im *NulBatchImageManager) CloseImage() error {
	fmt.Println("CloseImage called")
	im.file.Close()
	return nil
}
func (im *NulBatchImageManager) CloseDocumentBatch() error {
	fmt.Println("CloseDocumentBatch called")
	return nil
}

func NewNulBatchImageManager(docType string) (DocumentBatchHandler, error) {
	im := new(NulBatchImageManager)
	fmt.Println("New document batch with", docType)

	return DocumentBatchHandler(im), nil
}

func Test_ScanToPC(t *testing.T) {
	d, err := LocalizeDevice()
	if err == nil {
		ni := new(NulImageManager)
		fmt.Println("HPDevice found at", d.URL)
		d.NewScanJob(ni, "Platen", 200, "Gray")

	} else {
		fmt.Println("HPDevice error", err)
	}
}
