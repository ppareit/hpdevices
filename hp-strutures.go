// strutures.go
package hpdevices

import (
	"encoding/xml"
)

const xmlHeader = `<?xml version="1.0" encoding="utf-8"?>`

// XML structures used by the printer web interface

type discoveryTree struct {
	XMLName        xml.Name        `xml:"http://www.hp.com/schemas/imaging/con/ledm/2007/09/21 DiscoveryTree"`
	Revision       string          `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ Version>Revision"`
	Date           string          `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ Version>Date"`
	SupportedTrees []supportedTree `xml:"http://www.hp.com/schemas/imaging/con/ledm/2007/09/21 SupportedTree"`
	SupportedIfcs  []supportedIfc  `xml:"http://www.hp.com/schemas/imaging/con/ledm/2007/09/21 SupportedIfc"`
}

type supportedTree struct {
	XMLName      xml.Name //`xml:"http://www.hp.com/schemas/imaging/con/ledm/2007/09/21 SupportedTree"`
	ResourceURI  string   `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ ResourceURI"`
	ResourceType string   `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ ResourceType"`
	Revision     string   `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ Revision"`
}

type supportedIfc struct {
	XMLName      xml.Name `xml:"http://www.hp.com/schemas/imaging/con/ledm/2007/09/21 SupportedIfc"`
	ManifestURI  string   `xml:"http://www.hp.com/schemas/imaging/con/ledm/2007/09/21 ManifestURI"`
	ResourceType string   `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ ResourceType"`
	Revision     string   `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ Revision"`
}

type walkupScanToCompDestinations struct {
	XMLName                      xml.Name                      `xml:"http://www.hp.com/schemas/imaging/con/ledm/walkupscan/2010/09/28 WalkupScanToCompDestinations"`
	WalkupScanToCompDestinations []walkupScanToCompDestination `xml:"WalkupScanToCompDestination"`
}

type walkupScanToCompDestination struct {
	XMLName                  xml.Name                  `xml:"http://www.hp.com/schemas/imaging/con/ledm/walkupscan/2010/09/28 WalkupScanToCompDestination"`
	ResourceURI              string                    `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ ResourceURI"`
	Name                     string                    `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ Name"`
	Hostname                 string                    `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ Hostname"`
	LinkType                 string                    `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ LinkType"`
	WalkupScanToCompSettings *walkupScanToCompSettings `xml:"http://www.hp.com/schemas/imaging/con/ledm/walkupscan/2010/09/28 WalkupScanToCompSettings"`
}

type walkupScanToCompSettings struct {
	XMLName      xml.Name `xml:"http://www.hp.com/schemas/imaging/con/ledm/walkupscan/2010/09/28 WalkupScanToCompSettings"`
	ScanSettings scanType `xml:"http://www.hp.com/schemas/imaging/con/ledm/scantype/2008/03/17 ScanSettings"`
	Shortcut     string   `xml:"http://www.hp.com/schemas/imaging/con/ledm/walkupscan/2010/09/28 Shortcut"`
}

type postDestination struct {
	XMLName                  xml.Name                  `xml:"http://www.hp.com/schemas/imaging/con/ledm/walkupscan/2010/09/28 WalkupScanToCompDestination"`
	Name                     string                    `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ Name"`
	Hostname                 string                    `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/2009/04/06 Hostname"`
	LinkType                 string                    `xml:"http://www.hp.com/schemas/imaging/con/ledm/walkupscan/2010/09/28 LinkType"`
	WalkupScanToCompSettings *walkupScanToCompSettings `xml:"http://www.hp.com/schemas/imaging/con/ledm/walkupscan/2010/09/28 WalkupScanToCompSettings"`
}

type scanType struct {
	XMLName      xml.Name `xml:"http://www.hp.com/schemas/imaging/con/ledm/scantype/2008/03/17 ScanSettings"`
	ScanPlexMode string   `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ ScanPlexMode"`
}

type eventTable struct {
	XMLName  xml.Name `xml:"http://www.hp.com/schemas/imaging/con/ledm/events/2007/09/16 EventTable"`
	Revision string   `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ Version>Revision"`
	Date     string   `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ Version>Date"`
	Events   []event  `xml:"http://www.hp.com/schemas/imaging/con/ledm/events/2007/09/16 Event"`
}

type event struct {
	XMLName                  xml.Name  `xml:"http://www.hp.com/schemas/imaging/con/ledm/events/2007/09/16 Event"`
	UnqualifiedEventCategory string    `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ UnqualifiedEventCategory"`
	AgingStamp               string    `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ AgingStamp"`
	Payloads                 []payload `xml:"http://www.hp.com/schemas/imaging/con/ledm/events/2007/09/16 Payload"`
}

type payload struct {
	XMLName      xml.Name `xml:"http://www.hp.com/schemas/imaging/con/ledm/events/2007/09/16 Payload"`
	ResourceURI  string   `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ ResourceURI"`
	ResourceType string   `xml:"http://www.hp.com/schemas/imaging/con/dictionaries/1.0/ ResourceType"`
}

type scanSettings struct {
	XMLName            xml.Name `xml:"http://www.hp.com/schemas/imaging/con/cnx/scan/2008/08/19 ScanSettings"`
	XResolution        int
	YResolution        int
	XStart             int
	YStart             int
	Width              int
	Height             int
	Format             string
	CompressionQFactor int
	ColorSpace         string
	BitDepth           int
	InputSource        string
	GrayRendering      string
	ToneMap            toneMap
	SharpeningLevel    int
	NoiseRemoval       int
	ContentType        string
}

type toneMap struct {
	//	XMLName    xml.Name `xml:"http://www.hp.com/schemas/imaging/con/cnx/scan/2008/08/19 ToneMap"`
	XMLName    xml.Name `xml:"ToneMap"`
	Gamma      int      `xml:"Gamma"`
	Brightness int      `xml:"Brightness"`
	Contrast   int      `xml:"Contrast"`
	Highlite   int      `xml:"Highlite"`
	Shadow     int      `xml:"Shadow"`
	Threshold  int      `xml:"Threshold"`
}

type scanCap struct {
	XMLName          xml.Name     `xml:"http://www.hp.com/schemas/imaging/con/cnx/scan/2008/08/19 ScanCaps"`
	ModelName        string       `xml:"DeviceCaps>ModelName"`
	DerivativeNumber int          `xml:"DeviceCaps>DerivativeNumber"`
	ColorEntries     []colorEntry `xml:"ColorEntries>ColorEntry"`
	//TODO: Change following fields by a map
	Platen scanSource `xml:"Platen"`
	Adf    scanSource `xml:"Adf"`
}

type colorEntry struct {
	XMLName         xml.Name `xml:"ColorEntry"`
	ColorType       string   // K1,Gray8,Color8
	Formats         []string `xml:"Formats>Format"`                 //Raw,Jpeg
	ImageTransforms []string `xml:"ImageTransforms>ImageTransform"` //ToneMap,Sharpening,NoisRemoval
	GrayRenderings  []string `xml:"GrayRenderings>GrayRendering"`   //NTSC,GrayCcdEmulated
}

type scanSource struct {
	XMLName         xml.Name //`xml:`
	InputSourceCaps scanSourceCap
	FeederCapacity  int
	AdfOptions      []string `xml:"AdfOptions>AdfOption"`
}

type scanSourceCap struct {
	MinWidth              int
	MinHeight             int
	MaxWidth              int
	MaxHeight             int
	RiskyLeftMargin       int
	RiskyRightMargin      int
	RiskyTopMargin        int
	RiskyBottomMargin     int
	MinResolution         int
	MaxOpticalXResolution int
	MaxOpticalYResolution int
	SupportedResolutions  []resolution `xml:"SupportedResolutions>Resolution"`
}

type resolution struct {
	XResolution int      //75,100,200,300,600,1200,2400
	YResolution int      //75,100,200,300,600,1200,2400
	NumCcd      int      //1
	ColorTypes  []string `xml:"ColorTypes>ColorType"`
}

type walkupScanToCompEvent struct {
	XMLName                   xml.Name `xml:"http://www.hp.com/schemas/imaging/con/ledm/walkupscan/2010/09/28 WalkupScanToCompEvent"`
	WalkupScanToCompEventType string
}

type scanStatus struct {
	XMLName      xml.Name `xml:"http://www.hp.com/schemas/imaging/con/cnx/scan/2008/08/19 ScanStatus"`
	ScannerState string   // AdfError,BusyWithScanJob
	AdfState     string   // Empty,Loaded,Jammed
}

type job struct {
	XMLName        xml.Name `xml:"http://www.hp.com/schemas/imaging/con/ledm/jobs/2009/04/30 Job"`
	JobUrl         string
	JobCategory    string
	JobState       string //Canceled,Completed,Processing
	JobStateUpdate string
	ScanJob        scanJob `xml:"http://www.hp.com/schemas/imaging/con/cnx/scan/2008/08/19 ScanJob"`
}

type scanJob struct {
	XMLName      xml.Name `xml:"http://www.hp.com/schemas/imaging/con/cnx/scan/2008/08/19 ScanJob"`
	PreScanPage  *preScanPage
	PostScanPage *postScanPage
}

type preScanPage struct {
	XMLName          xml.Name `xml:"PreScanPage"`
	PageNumber       int
	PageState        string //PreparingScan
	BufferInfo       bufferInfo
	BinaryURL        string
	ImageOrientation string // Normal
}
type postScanPage struct {
	XMLName    xml.Name `xml:"PostScanPage"`
	PageNumber int
	PageState  string //UploadCompleted,CanceledByDevice
	TotalLines int
}

type bufferInfo struct {
	ScanSettings scanSettings
	ImageWidth   int
	ImageHeight  int
	BytesPerLine int
	Cooked       string //"Enabled"
}
