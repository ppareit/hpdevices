// ScanJob
package hpdevices

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

type ImageWriter interface {
	NewImageWriter() (io.WriteCloser, error)
}

type hpscanJob struct {
	Device      *HPDevice
	URL         string
	ImageWriter ImageWriter
	Http        http.Client
}

func defaultToneMapping() toneMap {
	return toneMap{Gamma: 1000,
		Brightness: 1000,
		Contrast:   1000,
		Highlite:   179,
		Shadow:     25,
		Threshold:  0,
	}
}
func defautScanSetting() scanSettings {
	return scanSettings{
		XResolution:        200,
		YResolution:        200,
		XStart:             0,
		YStart:             0,
		Width:              2481,
		Height:             3507,
		Format:             "Jpeg",
		CompressionQFactor: 0,
		ColorSpace:         "Gray",
		BitDepth:           8,
		InputSource:        "Platen",
		GrayRendering:      "NTSC",
		ToneMap:            defaultToneMapping(),
		SharpeningLevel:    0,
		NoiseRemoval:       0,
		ContentType:        "Document",
	}

}

func (d *HPDevice) NewScanJob(imagewriter ImageWriter, source string, resolution int, colorspace string) (err error) {
	sj := new(hpscanJob)
	sj.Device = d
	sj.ImageWriter = imagewriter

	ss := defautScanSetting()
	ss.XResolution, ss.YResolution = resolution, resolution
	ss.InputSource = string(source)
	ss.ColorSpace = colorspace

	buffer, err := xml.Marshal(ss)
	if err != nil {
		return NewHPDeviceError("HPDevice.ScanJob", "", err)
	}
	r := bytes.NewReader(append([]byte(xmlHeader), buffer...))

	resp, err := sj.Http.Post(d.URL+"/Scan/Jobs", "text/xml", r)
	if err != nil {
		return NewHPDeviceError("HPDevice.ScanJob", "POST", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return NewHPDeviceError("HPDevice.ScanJob", "/Scan/Jobs Post job unexpected status code"+resp.Status, nil)
	}
	sj.URL = resp.Header.Get("Location")
	resp.Body.Close()

	tick := time.NewTicker(10 * time.Second)
	for _ = range tick.C {

		resp, err := sj.Http.Get(sj.URL)
		if err != nil {
			return NewHPDeviceError("HPDevice.ScanJob", "PageLoop", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return NewHPDeviceError("HPDevice.ScanJob", "PageLoop Unexpected status "+resp.Status, nil)
		}
		j := new(job)
		buffer, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return NewHPDeviceError("HPDevice.ScanJob", "ScanJobLoop", err)
		}

		err = xml.Unmarshal(buffer, j)
		if err != nil {
			return NewHPDeviceError("HPDevice.ScanJob", "ScanJobLoop", err)
		}
		resp.Body.Close()

		Status := j.JobState
		switch Status {
		case "Processing":
			// During PreScan phase, check if a page is ready to upload
			if j.ScanJob.PreScanPage != nil && j.ScanJob.PreScanPage.PageState == "ReadyToUpload" {
				err = sj.DownloadImage(sj.Device.URL+j.ScanJob.PreScanPage.BinaryURL, j.ScanJob.PreScanPage.BufferInfo.ImageHeight)
				if err != nil {
					return NewHPDeviceError("HPDevice.ScanJob", "DownloadImage", err)
				}
			}
			// During PostScan phase, check if job is not canceled
			if j.ScanJob.PostScanPage != nil && j.ScanJob.PostScanPage.PageState == "CanceledByDevice" {
				return NewHPDeviceError("ScanBatch.ScanJobLoop", "CanceledByDevice", nil)
			}
		case "Canceled":
			return NewHPDeviceError("ScanBatch.ScanJobLoop", "Canceled status", nil)
		case "Completed":
			return nil
		}
	}

	return nil
}

func (sj *hpscanJob) DownloadImage(image_url string, image_height int) (err error) {

	resp, err := sj.Http.Get(image_url)
	if err != nil {
		return NewHPDeviceError("ScanJob.DownloadImage", "Get "+image_url, err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return NewHPDeviceError("ScanJob.DownloadImage", "Unexpected status "+resp.Status, nil)
	}

	writer, err := sj.ImageWriter.NewImageWriter()

	_, err = sj.FixJPEG(writer, resp.Body, image_height)
	if err != nil {
		return NewHPDeviceError("ScanJob.DownloadImage", "Error during FixJPEG ", err)
	}

	err = writer.Close()
	if err != nil {
		return NewHPDeviceError("ScanJob.DownloadImage", "Error when closing writer", err)
	}
	return err

}

/* jpegfix gets jpeg stream delivered by HP scanner when using ADF
Segment DCT 0xFFC0 has a bad Lines field.

*/

func (sj *hpscanJob) FixJPEG(w io.Writer, r io.Reader, ActualLineNumber int) (written int64, err error) {
	i := 0
	buf := make([]byte, 256)
	l, err := r.Read(buf)
	if l != len(buf) || (buf[0] != 0xff && buf[1] != 0xd8) {
		return 0, errors.New("Not a JPEG stream")
	}
	i = 2
	for i < len(buf) && buf[i] == 0xff && buf[i+1] != 0xc0 {
		size := int(buf[i+2])<<8 + int(buf[i+3]) + 2
		if size < 0 {
			return 0, errors.New("Frame lengh invalid")
		}
		i += size
	}
	if i >= len(buf) || (buf[i] != 0xff && buf[i+1] != 0xc0) {
		return 0, errors.New("SOF marker not found in the header")
	}

	// Fix line number
	i += 5
	if buf[i] == 0xff && buf[i+1] == 0xff {
		// Affected image
		buf[i] = byte(ActualLineNumber >> 8)
		buf[i+1] = byte(ActualLineNumber & 0x00ff)
	}
	// Write header
	l, err = w.Write(buf)
	written += int64(l) /* jpegfix gets jpeg stream delivered by HP scanner when using ADF
	Segment DCT 0xFFC0 has a bad Lines field.

	*/

	// stream
	if l == len(buf) && err == nil {
		buf = make([]byte, 32*1024)
		for {
			nr, er := r.Read(buf)
			if nr > 0 {
				nw, ew := w.Write(buf[0:nr])
				if nw > 0 {
					written += int64(nw)
				}
				if ew != nil {
					err = ew
					break
				}
				if nr != nw {
					err = io.ErrShortWrite
					break
				}
			}
			if er == io.EOF {
				break
			}
			if er != nil {
				err = er
				break
			}
		}
	}
	return written, err
}

func (sj *hpscanJob) GetStatus() (*scanStatus, error) {
	var ScanStatus *scanStatus
	resp, err := sj.Http.Get(sj.Device.URL + "/Scan/Status")
	if err != nil {
		err = NewHPDeviceError("hpscanJob.GetStatus", "get", err)
	}

	if err == nil && resp.StatusCode != 200 {
		err = NewHPDeviceError("hpscanJob.GetStatus", "Unexpected status"+resp.Status, err)
		resp.Body.Close()
	}

	buffer, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		err = NewHPDeviceError("hpscanJob.GetStatus", "ReadAll", err)
	}

	if err == nil {
		ScanStatus := new(scanStatus)
		err = xml.Unmarshal(buffer, ScanStatus)
	}
	if err != nil {
		err = NewHPDeviceError("hpscanJob.GetStatus", "Unmarshal", err)
	}
	return ScanStatus, err
}

func (sj *hpscanJob) GetSource() (source string, err error) {

	scanStatus, err := sj.GetStatus()
	if err == nil {
		if scanStatus.AdfState == "Empty" {
			source = "Platen"
		} else {
			source = "Adf"
		}
		return source, err
	}
	return "", err
}
