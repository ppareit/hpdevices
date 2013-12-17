package hpdevices

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

const (
	destinationTimeOut = 3 * time.Minute      // 30 minutes
	eventLoopTimeOut   = 2 * time.Minute / 30 // 2 minutes
)

type DestinationSettings struct {
	Name        string
	FilePattern *string
	DoOCR       bool // True when OCR must be performed
	Verso       bool // True when the current job should be merged with previous to become the second side
	Resolution  int
	ColorSpace  string
}

type DocumentBatchHandlerFactory func(docType string) (DocumentBatchHandler, error)

type DocumentBatchHandler interface {
	NewImageWriter() (io.WriteCloser, error)
	CloseImage() error
	CloseDocumentBatch() error
}

type AgingStamp struct {
	i int
	j int
}

type hpscanToPC struct {
	Device                      *HPDevice
	DocumentBatchHandlerFactory DocumentBatchHandlerFactory
	Destinations                map[string]DestinationSettings
	Http                        http.Client
	AgingStamp                  AgingStamp
	DocumentBatchHandler        DocumentBatchHandler
}

// NewScanToPC: Create a structure, register destinations and launch event loop
// returns when connection is dropped or when an error has occured

func NewScanToPC(Device *HPDevice, documentBatchHandlerFactory DocumentBatchHandlerFactory, HostName string, Destinations []DestinationSettings) (stp *hpscanToPC, err error) {

	stp = new(hpscanToPC)
	stp.Device = Device
	stp.DocumentBatchHandlerFactory = documentBatchHandlerFactory
	stp.Destinations = make(map[string]DestinationSettings)

	err = stp.MainLoop(HostName, Destinations)

	return stp, err
}

//Register: Register destinations on the device

func (stp *hpscanToPC) Register(HostName string, Destinations []DestinationSettings) (err error) {
	for _, Destination := range Destinations {
		hpdestination := &postDestination{
			Name:     HostName + "(" + Destination.Name + ")",
			Hostname: HostName + "(" + Destination.Name + ")",
			LinkType: "Network",
		}
		buffer, err := xml.Marshal(hpdestination)
		if err != nil {
			return NewHPDeviceError("hpscanToPC.Register", "Post", err)
		}

		r := bytes.NewReader(append([]byte(xmlHeader), buffer...))
		resp, err := stp.Http.Post(stp.Device.URL+"/WalkupScanToComp/WalkupScanToCompDestinations", "text/xml", r)
		if err != nil {
			return NewHPDeviceError("hpscanToPC.Register", "POST", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 201 {
			return NewHPDeviceError("hpscanToPC.Register", "Unexpected Status "+resp.Status, err)
		}
		// SuccessFull registration
		uri := resp.Header.Get("Location")
		uuid := getUUIDfromURI(uri)
		stp.Destinations[uuid] = Destination // Link uuid with settings
	}
	return nil
}

/* Mainloop: ScantoPC event loop. Will end when the connection is dropped or when an error has occured
The loop takes care of following:
- Periodicaly reregister destinations. This will ensure the destinations are placed on top of destination list on the printer
- Wait device events in efficient way, using timout parameter. Then event pulling wait until someting happens on the device
	Events are sent to a channel from a go routine
- If an error occurs into the event loop or if timeout requiers to kill the event loop, send the  stop instruction to the loop and waits it's actually done
	This prevent nasty bugs with several event loops runing concurently
*/
func (stp *hpscanToPC) MainLoop(HostName string, Destinations []DestinationSettings) (err error) {

	// Innital step:
	//	- register destinations
	//	- Initiate event loop

	err = stp.Register(HostName, Destinations)
	if err != nil {
		return err
	}
	eventsChannel := make(chan *eventTable)
	errorsChannel := make(chan error)
	stopChannel := make(chan chan bool)
	err = stp.NewEventLoop(eventsChannel, errorsChannel, stopChannel)
	if err == nil {
		var timer time.Timer
		// The loop
		for {
			select {
			case eventTable := <-eventsChannel:
				err = stp.ParseEventTable(eventTable)
			case <-timer.C:
				closedChannel := make(chan bool) // Prepares the closing confirmation channel
				stopChannel <- closedChannel     // Send the stop instruction with the the closing confirmation
				<-closedChannel                  // Wait the confirmation of closed state of the event loop
				err = stp.Register(HostName, Destinations)
				if err == nil {
					// Start a new event loop with the new destination
					err = stp.NewEventLoop(eventsChannel, errorsChannel, stopChannel)
				}
			case err = <-errorsChannel: // Get errors occurred in the event loop. The event loop is already closed

			}
			if err != nil {
				return err
			}
		}
	}
	return err
}

func (stp *hpscanToPC) NewEventLoop(eventsChannel chan *eventTable, errorsChannel chan error, stopChannel chan chan bool) (err error) {
	go stp.EventLoop(eventsChannel, errorsChannel, stopChannel)
	return nil
}

func (stp *hpscanToPC) EventLoop(eventsChannel chan *eventTable, errorsChannel chan error, stopChannel chan chan bool) {
	var err error

	// Prepare an HTTP connection with a custom timeout
	timeoutClient := newTimeoutClient(2*time.Second, 2*eventLoopTimeOut/3) // 1.5 * HP device timeout
	// on call to get firts events and e-tag
	resp, err := stp.Http.Get(stp.Device.URL + "/EventMgmt/EventTable")
	if err != nil {
		err = NewHPDeviceError("hpscanToPC.EventLoop", "", err)
	}

	defer resp.Body.Close()
	if err == nil && resp.StatusCode != 200 {
		err = NewHPDeviceError("hpscanToPC.EventLoop", "Unexpected Status "+resp.Status, err)
	}

	Etag := resp.Header.Get("Etag") // Get the 1st Etag for the lop
	et := new(eventTable)
	buffer, err := ioutil.ReadAll(resp.Body)
	err = xml.Unmarshal(buffer, et)
	if err != nil {
		err = NewHPDeviceError("hpscanToPC.EventLoop", "Marshal", err)
	}
	resp.Body.Close()

	if err == nil {
		eventsChannel <- et

		// Event Loop while no error
		for err == nil {
			// Watch stopChannel
			select {
			case okChan := <-stopChannel: // I need to quit....
				//TODO: kill the current connection. What happen if stops occurs in same time of user event on the device

				okChan <- true // Send the information that I have quitted
				return
			default:
				// Don't block the loop
			}
			request, err := http.NewRequest("GET", stp.Device.URL+"/EventMgmt/EventTable?timeout="+fmt.Sprintf("%d", int(eventLoopTimeOut.Seconds())*10), nil)
			if err != nil {
				err = NewHPDeviceError("hpscanToPC.EventLoop", "NewRequest", err)
			}
			if err == nil {
				request.Header.Add("If-None-Match", Etag) // Tell to the device which event we already know
				resp, err = timeoutClient.Do(request)
				if err != nil {
					err = NewHPDeviceError("hpscanToPC.EventLoop", "get", err)
				}
			}
			if err == nil {
				switch resp.StatusCode {
				case 304: // Nothing new since last call
					resp.Body.Close()
				case 200: // Something happened
					Etag = resp.Header.Get("Etag") // Preserve Etag for the next call to the device
					et = new(eventTable)
					buffer, err = ioutil.ReadAll(resp.Body)
					resp.Body.Close()
					if err != nil {
						err = NewHPDeviceError("hpscanToPC.EventLoop", "ReadAll", err)
					}
					if err == nil {
						err = xml.Unmarshal(buffer, et)
						if err != nil {
							err = NewHPDeviceError("hpscanToPC.EventLoop", "Marshal", err)
						}
						if err == nil {
							eventsChannel <- et // Send the event table to main loop
						}
					}
				default:
					resp.Body.Close()
					err = NewHPDeviceError("hpscanToPC.EventLoop", "Unexpected status"+resp.Status)
				}
			}
		}
	}
	// If leaving the loop because on an error, send it to the main loop before closing the go routine
	if err != nil {
		errorsChannel <- err
	}

}

// HTTP clients with custom timeout
func timeoutDialer(cTimeout time.Duration, rwTimeout time.Duration) func(net, addr string) (c net.Conn, err error) {
	return func(netw, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(netw, addr, cTimeout)
		if err != nil {
			return nil, err
		}
		conn.SetDeadline(time.Now().Add(rwTimeout))
		return conn, nil
	}
}
func newTimeoutClient(connectTimeout time.Duration, readWriteTimeout time.Duration) *http.Client {

	return &http.Client{
		Transport: &http.Transport{
			Dial: timeoutDialer(connectTimeout, readWriteTimeout),
		},
	}
}

func (stp *hpscanToPC) ParseEventTable(eventTable *eventTable) (err error) {
	// Parse evenCompletet list ScanEvent
	for _, event := range eventTable.Events {
		switch event.UnqualifiedEventCategory {
		case "ScanEvent":
			err = stp.ScanEvent(event)
		case "PoweringDownEvent":
			//TODO: Close pending ScanBatch
			err = NewHPDeviceError("hpscanToPC.ParseEventTable", "Powerdown event recieved", nil)
		default:
			// Ignore silently all other events
		}
		if err != nil { // Break the loop when error
			return err
		}
	}
	return nil
}

func (stp *hpscanToPC) ScanEvent(e event) (err error) {
	var a AgingStamp
	n, err := fmt.Sscanf(e.AgingStamp, "%d-%d", &a.i, &a.j) // AgingStamp is like 48-189
	if err != nil || n != 2 {
		err = NewHPDeviceError("hpscanToPC.ScanEvent", "Unexpected error", err)
	}
	// Check we have something really new
	if (a.i > stp.AgingStamp.i) || (a.i == stp.AgingStamp.i && a.j > stp.AgingStamp.i) {
		uri := ""
		for _, payload := range e.Payloads {
			switch payload.ResourceType {
			case "wus:WalkupScanToCompDestination":
				uri = payload.ResourceURI
			}
		}
		// Check if the WalkupScanToComp event is for one of our destinations
		if dest, ok := stp.Destinations[getUUIDfromURI(uri)]; ok {
			walkupScanToCompDestination, err := stp.GetWalkupScanToCompDestinations(uri)
			if err == nil {
				stp.AgingStamp = a
				err = stp.WalkupScanToCompEvent(&dest, walkupScanToCompDestination)
			}
		}
	}
	return err
}

func (stp *hpscanToPC) GetWalkupScanToCompDestinations(uri string) (*walkupScanToCompDestination, error) {
	var dest *walkupScanToCompDestination
	resp, err := stp.Http.Get(stp.Device.URL + "/WalkupScanToComp/WalkupScanToCompDestinations")
	if err != nil {
		err = NewHPDeviceError("hpscanToPC.WalkupScanToCompDestinations", "", err)
	}
	if err != nil && resp.StatusCode != 200 {
		resp.Body.Close()
		err = NewHPDeviceError("hpscanToPC.WalkupScanToCompDestinations", "Unexpected Status "+resp.Status, err)
	}
	if err == nil {
		buffer, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			err = NewHPDeviceError("hpscanToPC.WalkupScanToCompDestinations", "ReadAll", err)
		}

		dest := new(walkupScanToCompDestination)
		err = xml.Unmarshal(buffer, dest)

		// Call the given URI
		resp, err = stp.Http.Get(stp.Device.URL + uri)

		if err != nil {
			err = NewHPDeviceError("hpscanToPC.WalkupScanToCompDestinations", "Get "+uri, err)
		}

		if err == nil && resp.StatusCode != 200 {
			resp.Body.Close()
			err = NewHPDeviceError("hpscanToPC.WalkupScanToCompDestinations", "Unexpected Status "+resp.Status, err)
		}

		if err == nil {
			buffer, err = ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				err = NewHPDeviceError("hpscanToPC.WalkupScanToCompDestinations", "ReadAll", err)
			}
			if err == nil {
				dest = new(walkupScanToCompDestination)
				err = xml.Unmarshal(buffer, dest)
				if err != nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompDestinations", "Unmarshal", err)
				}
			}
		}
	}
	return dest, err
}

func (stp *hpscanToPC) WalkupScanToCompEvent(Destination *DestinationSettings, walkupScanToCompDestination *walkupScanToCompDestination) error {
	// Handle a scan event
	resp, err := stp.Http.Get(stp.Device.URL + "/WalkupScanToComp/WalkupScanToCompEvent")
	if err != nil {
		err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "", err)
	}

	if err == nil && resp.StatusCode != 200 {
		resp.Body.Close()
		err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "Unexpected Status "+resp.Status, err)
	}
	if err == nil {
		event := new(walkupScanToCompEvent)
		buffer, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "ReadAll", err)
		}
		resp.Body.Close()
		if err == nil {
			err = xml.Unmarshal(buffer, event)
		}
		if err != nil {
			err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "Unmarshal", err)
		}
		if err == nil {
			switch event.WalkupScanToCompEventType {
			case "HostSelected": // That's for us...
				time.Sleep(1 * time.Second / 2) // Test: sometime, the device doesn't provide HPWalkupScanToCompDestination in next event

			case "ScanRequested": // Start Adf scanning or 1st page on Platen scanning
				//if stp.DocumentBatchHandler  == nil {
				//	err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "recieved ScanRequested, but DocumentBatchHandlerFactory is nil", nil)
				//}

				if walkupScanToCompDestination == nil || walkupScanToCompDestination.WalkupScanToCompSettings == nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "recieved ScanRequested, HPWalkupScanToCompDestination nil?", nil)
				}
				stp.DocumentBatchHandler, err = stp.DocumentBatchHandlerFactory(walkupScanToCompDestination.WalkupScanToCompSettings.Shortcut[4:])
				if err != nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "DocumentBatchHandlerFactory", err)
				}
				//TODO: ScanSource
				err = stp.Device.NewScanJob(stp.DocumentBatchHandler, "Platen", Destination.Resolution, Destination.ColorSpace)
				if err != nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "ScanRequested/NewScanJob", err)
				}

			case "ScanNewPageRequested": //Subsequent pages on Platen
				if stp.DocumentBatchHandler == nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "recieved ScanNewPageRequested, but DocumentBatchHandlerFactory is nil", nil)
				}
				//TODO: ScanSource
				err = stp.Device.NewScanJob(stp.DocumentBatchHandler, "Platen", Destination.Resolution, Destination.ColorSpace)
				if err != nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "ScanNewPageRequested/NewScanJob", err)
				}

			case "ScanPagesComplete": //End of ScanBatch
				if stp.DocumentBatchHandler == nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "recieved ScanPagesComplete, but DocumentBatchHandlerFactory is nil", nil)
				}
				err = stp.DocumentBatchHandler.CloseDocumentBatch()
				if err != nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "ScanPagesComplete/CloseDocumentBatch", err)
				}

			default:
				err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "Unknown event"+event.WalkupScanToCompEventType, nil)
			}
		}
	}
	return err

}
