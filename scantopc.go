package hpdevices

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
)

var (
	TRACE   *log.Logger
	INFO    *log.Logger
	WARNING *log.Logger
	ERROR   *log.Logger
)

func InitLogger(traceHandle *log.Logger,
	infoHandle *log.Logger,
	warningHandle *log.Logger,
	errorHandle *log.Logger) {

	TRACE = traceHandle
	INFO = infoHandle
	WARNING = warningHandle
	ERROR = errorHandle
}

const (
	destinationTimeOut = 30 * time.Minute // 30 minutes
	eventLoopTimeOut   = 2 * time.Minute  // 2 minutes
)

type DestinationSettings struct {
	Name        string
	FilePattern *string
	DoOCR       bool // True when OCR must be performed
	Verso       bool // True when the current job should be merged with previous to become the second side
	Resolution  int
	ColorSpace  string
}

type DocumentBatchHandlerFactory func(doctype string, destination *DestinationSettings, format string, previousbatch DocumentBatchHandler) (DocumentBatchHandler, error)

type DocumentBatchHandler interface {
	NewImageWriter() (io.WriteCloser, error)
	CloseDocumentBatch() error
}

type AgingStamp struct {
	i int
	j int
}

var agingStamp AgingStamp // keep last event seen to discard old and duplicates

type hpscanToPC struct {
	Device                      *HPDevice                      // The device properties
	DocumentBatchHandlerFactory DocumentBatchHandlerFactory    // Function used to generate document manager
	DocumentBatchHandler        DocumentBatchHandler           // current and previous document batches
	Destinations                map[string]DestinationSettings // Key is UUID delivered by the device
	scanSource                  string                         // Scan source : Platen,Adf
}

// NewScanToPC: Create a structure, register destinations and launch event loop
// returns when connection is dropped or when an error has occured

func NewScanToPC(Device *HPDevice, documentBatchHandlerFactory DocumentBatchHandlerFactory, HostName string, Destinations []DestinationSettings) (stp *hpscanToPC, err error) {
	TRACE.Println("NewScanToPC")
	stp = new(hpscanToPC)
	stp.Device = Device
	stp.DocumentBatchHandlerFactory = documentBatchHandlerFactory
	stp.Destinations = make(map[string]DestinationSettings)

	err = stp.MainLoop(HostName, Destinations)
	TRACE.Println("Exit ScanToPC with error", err)

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
		resp, err := http.Post(stp.Device.URL+"/WalkupScanToComp/WalkupScanToCompDestinations", "text/xml", r)
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
		TRACE.Println("hpscanToPC.Register : New destination", uuid, uri)
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

var mainLoopCount = 0

func (stp *hpscanToPC) MainLoop(HostName string, Destinations []DestinationSettings) (err error) {

	// Inital step:
	//	- register destinations
	//	- Initiate event loop
	mainLoopCount++

	TRACE.Println("Enter in MainLoop #", mainLoopCount)
	defer TRACE.Println("Exit MainLoop #", mainLoopCount)
	err = stp.Register(HostName, Destinations)
	if err != nil {
		return err
	}
	eventsChannel := make(chan *eventTable)
	errorsChannel := make(chan error)
	stopChannel := make(chan chan bool)

	err = stp.NewEventLoop(eventsChannel, errorsChannel, stopChannel)
	if err == nil {
		timer := time.NewTimer(destinationTimeOut)
		// The loop
		for {
			select {
			case eventTable := <-eventsChannel: // Get event table from HTTP query
				err = stp.ParseEventTable(eventTable)
				timer = time.NewTimer(destinationTimeOut)
			case <-timer.C:
				TRACE.Println("hpscanToPC.MainLoop: Time to register again")
				closedChannel := make(chan bool) // Prepares the closing confirmation channel
				stopChannel <- closedChannel     // Send the stop instruction with the the closing confirmation
				<-closedChannel                  // Wait the confirmation of closed state of the event loop
				err = stp.Register(HostName, Destinations)
				if err == nil {
					// Start a new event loop with the new destination
					err = stp.NewEventLoop(eventsChannel, errorsChannel, stopChannel)
					timer = time.NewTimer(destinationTimeOut)
				}
			case err = <-errorsChannel: // Get errors occurred in the event loop. The event loop is already closed
				fmt.Println("Recieve error from event loop")
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

var EventLoopCount = 0

func (stp *hpscanToPC) EventLoop(eventsChannel chan *eventTable, errorsChannel chan error, stopChannel chan chan bool) {
	var err error
	EventLoopCount++
	var elc = EventLoopCount

	TRACE.Println("Start EventLoop #", elc)
	defer TRACE.Println("Stop EventLoop #", elc)

	// on call to get firts events and e-tag
	resp, err := http.Get(stp.Device.URL + "/EventMgmt/EventTable")
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
		eventsChannel <- et // Send firsts events
		var (
			timeoutClient *http.Client
			request       *http.Request
			response      *http.Response
		)
		// Event Loop while no error
		for err == nil {
			// Watch stopChannel
			select {
			case okChan := <-stopChannel: // I need to quit....
				//TODO: kill the current connection. What happen if stops occurs in same time of user event on the device

				TRACE.Println("Quitting hpscanToPC.event loop")

				okChan <- true // Send the information that I have quitted
				return
			default:
				timeoutClient = newTimeoutClient(2*time.Second, eventLoopTimeOut+10*time.Second) // 2 sec for the header, 1.5 * HP device timeout for getting the boddy
				request, err = http.NewRequest("GET", stp.Device.URL+"/EventMgmt/EventTable?timeout="+fmt.Sprintf("%d", int(eventLoopTimeOut.Seconds())*10), nil)
				if err != nil {
					err = NewHPDeviceError("hpscanToPC.EventLoop", "NewRequest", err)
				}
				if err == nil {
					request.Header.Add("If-None-Match", Etag) // Tell to the device which event we already know
					request.Close = true                      // For closing the connection after having recieved the answer.
					response, err = timeoutClient.Do(request)
				}
				if err != nil {
					err = NewHPDeviceError("hpscanToPC.EventLoop", "request", err)
				}
				if err == nil {
					// The response
					switch response.StatusCode {
					case 304: // Nothing new since last call
						response.Body.Close()
						TRACE.Println("EventLoop #", elc, "no event...")
					case 200: // Something happened
						Etag = response.Header.Get("Etag") // Preserve Etag for the next call to the device
						et = new(eventTable)
						buffer, err = ioutil.ReadAll(response.Body)
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
								TRACE.Println("EventLoop #", elc, "event...")
								eventsChannel <- et // Send the event table to main loop
							}
						}
					default:
						response.Body.Close()
						err = NewHPDeviceError("hpscanToPC.EventLoop", "Unexpected status"+response.Status)
					}
				}
			}
		}
	}
	// If leaving the loop because of an error, send it to the main loop before closing the go routine
	if err != nil {
		fmt.Println("Error detected", err)
		errorsChannel <- err
	}
}

func (stp *hpscanToPC) GetNewEvent(client *http.Client, req *http.Request, respChan chan *http.Response, errChan chan error) {
	fmt.Println("(hpscanToPC) GetNewEvent query")
	req.Close = true
	resp, err := client.Do(req)
	if err != nil {
		errChan <- err
		return
	}

	fmt.Println("(hpscanToPC) GetNewEvent send response")
	respChan <- resp
	fmt.Println("hpscanToPC) GetNewEvent exit")
	return
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

func (stp *hpscanToPC) ParseEventTable(et *eventTable) (err error) {
	TRACE.Println("hpscanToPC.ParseEventTable", len(et.Events))
	// Parse evenCompletet list ScanEvent
	for _, event := range et.Events {
		TRACE.Println("hpscanToPC.ParseEventTable", "UnqualifiedEventCategory", event.UnqualifiedEventCategory)
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
	TRACE.Println("hpscanToPC.ScanEvent", "Get AgingStamp", e.AgingStamp)
	n, err := fmt.Sscanf(e.AgingStamp, "%d-%d", &a.i, &a.j) // AgingStamp is like 48-189
	if err != nil || n != 2 {
		err = NewHPDeviceError("hpscanToPC.ScanEvent", "Incorrert format AgingStamp "+e.AgingStamp, err)
		TRACE.Println("hpscanToPC.ScanEvent", "Incorrert format AgingStamp "+e.AgingStamp)
	} else {
		TRACE.Printf("%s %s %+v %s %+v", "hpscanToPC.ScanEvent", "Memorized", agingStamp, "got", a)
		if (a.i > agingStamp.i) || (a.i == agingStamp.i && a.j > agingStamp.j) {
			// Check we have something really new
			agingStamp = a // Keep last event handled
			TRACE.Println("hpscanToPC.ScanEvent", "Handling AgingStamp", e.AgingStamp)
			uri := ""
			TRACE.Println("hpscanToPC.ScanEvent", "e.Payloads", len(e.Payloads))
			for i, payload := range e.Payloads {
				TRACE.Println("hpscanToPC.ScanEvent", i, payload.ResourceType, payload.ResourceURI)
				switch payload.ResourceType {
				case "wus:WalkupScanToCompDestination":
					uri = payload.ResourceURI
				}
			}
			TRACE.Println("hpscanToPC.ScanEven event uri", uri)
			// Check if the WalkupScanToComp event is for one of our destinations
			if dest, ok := stp.Destinations[getUUIDfromURI(uri)]; ok {
				walkupScanToCompDestination, err := stp.GetWalkupScanToCompDestinations(uri)
				if err == nil {
					err = stp.WalkupScanToCompEvent(&dest, walkupScanToCompDestination)
				}
			}
		} else {
			TRACE.Println("hpscanToPC.ScanEvent", "AgingStamp already handled", e.AgingStamp)
		}
	}
	return err
}

func (stp *hpscanToPC) GetWalkupScanToCompDestinations(uri string) (*walkupScanToCompDestination, error) {
	TRACE.Println("hpscanToPC.WalkupScanToCompDestinations entering")
	var dest *walkupScanToCompDestination
	//TODO: Is this call absolutly necessaire?
	resp, err := http.Get(stp.Device.URL + "/WalkupScanToComp/WalkupScanToCompDestinations")
	if err != nil {
		err = NewHPDeviceError("hpscanToPC.WalkupScanToCompDestinations", "", err)
	}

	if err == nil && resp.StatusCode != 200 {
		resp.Body.Close()
		err = NewHPDeviceError("hpscanToPC.WalkupScanToCompDestinations", "Unexpected Status "+resp.Status, err)
	}
	if err == nil {
		buffer, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			err = NewHPDeviceError("hpscanToPC.WalkupScanToCompDestinations", "ReadAll", err)
		}

		destinations := new(walkupScanToCompDestinations)
		err = xml.Unmarshal(buffer, destinations)
		if err != nil {
			err = NewHPDeviceError("hpscanToPC.WalkupScanToCompDestinations", "Unmarshal", err)
		}
		have_a_match := false
		for _, d := range destinations.WalkupScanToCompDestinations {
			if d.ResourceURI == uri {
				have_a_match = true
			}
		}

		TRACE.Println("hpscanToPC.WalkupScanToCompDestinations have_a_match", have_a_match)

		if err == nil {
			// Call the given URI
			resp, err = http.Get(stp.Device.URL + uri)
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
	TRACE.Println("hpscanToPC.WalkupScanToCompDestinations", dest.Name, dest.WalkupScanToCompSettings)
	return dest, err
}

func (stp *hpscanToPC) WalkupScanToCompEvent(Destination *DestinationSettings, walkupScanToCompDestination *walkupScanToCompDestination) error {
	// Handle a scan event
	TRACE.Println("hpscanToPC.WalkupScanToCompEvent", "entering")
	resp, err := http.Get(stp.Device.URL + "/WalkupScanToComp/WalkupScanToCompEvent")
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
		TRACE.Println("hpscanToPC.WalkupScanToCompEvent", event.WalkupScanToCompEventType)
		if err != nil {
			err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "Unmarshal", err)
		}
		if err == nil {
			switch event.WalkupScanToCompEventType {
			case "HostSelected": // That's for us...
				stp.scanSource, err = stp.Device.GetSource()
				TRACE.Println("hpscanToPC.WalkupScanToCompEvent", stp.scanSource, err)
				//time.Sleep(1 * time.Second / 3) // Test: sometime, the device doesn't provide HPWalkupScanToCompDestination in next event

			case "ScanRequested": // Start Adf scanning or 1st page on Platen scanning
				//if stp.DocumentBatchHandler  == nil {
				//	err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "recieved ScanRequested, but DocumentBatchHandlerFactory is nil", nil)
				//}
				TRACE.Println("Mainloop", mainLoopCount, "ScanRequested")
				if walkupScanToCompDestination == nil || walkupScanToCompDestination.WalkupScanToCompSettings == nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "recieved ScanRequested, HPWalkupScanToCompDestination nil?", nil)
				}
				stp.DocumentBatchHandler, err = stp.DocumentBatchHandlerFactory(walkupScanToCompDestination.WalkupScanToCompSettings.Shortcut[4:], Destination, "Jpeg", stp.DocumentBatchHandler)
				if err != nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "DocumentBatchHandlerFactory", err)
				}
				//TODO: ScanSource
				err = stp.Device.NewScanJob(stp.DocumentBatchHandler, stp.scanSource, Destination.Resolution, Destination.ColorSpace)
				if err != nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "ScanRequested/NewScanJob", err)
				}

			case "ScanNewPageRequested": //Subsequent pages on Platen
				TRACE.Println("Mainloop", mainLoopCount, "ScanNewPageRequested")
				if stp.DocumentBatchHandler == nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "recieved ScanNewPageRequested, but DocumentBatchHandlerFactory is nil", nil)
				}
				//TODO: ScanSource
				err = stp.Device.NewScanJob(stp.DocumentBatchHandler, stp.scanSource, Destination.Resolution, Destination.ColorSpace)
				if err != nil {
					err = NewHPDeviceError("hpscanToPC.WalkupScanToCompEvent", "ScanNewPageRequested/NewScanJob", err)
				}

			case "ScanPagesComplete": //End of ScanBatch
				TRACE.Println("Mainloop", mainLoopCount, "ScanPagesComplete")
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
