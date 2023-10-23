package components

// hvd
import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/joy4/av/avutil"
	"github.com/kerberos-io/joy4/av/pubsub"
	"github.com/kerberos-io/joy4/format/rtmp"
)

type STATE int

const (
	BEGIN STATE = iota
	STARTING
	STARTED
	NOTSTARTED
	STOPPED
	ERROR
	ERRORSTREAM
	ERRORCONNECT
)

func (s STATE) String() string {
	switch s {
	case BEGIN:
		return "Begin"
	case STARTING:
		return "Starting"
	case STARTED:
		return "Started"
	case NOTSTARTED:
		return "NotStarted"
	case STOPPED:
		return "Stopped"
	case ERROR:
		return "Error"
	case ERRORSTREAM:
		return "ErrStream"
	case ERRORCONNECT:
		return "ErrConnect"
	default:
		return "Unknown"
	}
}

func connectWithTimeout(req models.LiveRestreamReq, timeout time.Duration) (*rtmp.Conn, error) {
	var err error
	var conn *rtmp.Conn = nil
	timeAfter := time.After(timeout)

	for {
		select {
		case <-timeAfter:
			log.Log.Info("Connecting timeout!")
			return nil, err
		default:
			conn, err = rtmp.Dial(req.ProxyAddr)
			if err == nil {
				return conn, nil
			} else {
				log.Log.Error("Connect error: " + err.Error())
				time.Sleep(1000 * time.Millisecond)
				log.Log.Info("Retry connect...")
			}
		}
	}
}

func isStarted(currenState STATE) bool {
	return currenState == STARTING || currenState == STARTED
}

func mqttRespTopic(configuration *models.Configuration) string {
	config := configuration.Config
	key := ""
	if config.Cloud == "s3" && config.S3 != nil && config.S3.Publickey != "" {
		key = config.S3.Publickey
	} else if config.Cloud == "kstorage" && config.KStorage != nil && config.KStorage.CloudKey != "" {
		key = config.KStorage.CloudKey
	}
	// This is the new way ;)
	if config.HubKey != "" {
		key = config.HubKey
	}

	topic := "kerberos/" + key + "/device/" + config.Key + "/livestream/resp"

	return topic
}

func mqttPublish(resp *models.LiveRestreamResp, configuration *models.Configuration, mqttClient *mqtt.Client) {
	respbytes, _ := json.Marshal(*resp)
	topic := mqttRespTopic(configuration)
	(*mqttClient).Publish(topic, 2, false, respbytes)
}

func CopyStream(dst *rtmp.Conn, src *pubsub.QueueCursor, monitorChan chan string) {
	defer func() {
		if recover() != nil {
			log.Log.Error("Channel closed.")
		}
	}()

	// streams, _ := src.Streams()
	// log.Log.Info(fmt.Sprintf("Streams: %+v \n", streams))

	if err := avutil.CopyFile(dst, src); err != nil {
		monitorChan <- "closed:" + err.Error()
	} else {
		monitorChan <- "closed:CopyStream graceful stop"
	}
}

func handle(reqChan chan models.LiveRestreamReq, monitorChan chan string, streamCursor *pubsub.QueueCursor, configuration *models.Configuration,
	mqttClient mqtt.Client, mutex *sync.Mutex, wg *sync.WaitGroup) {

	defer wg.Done()

	var proxyConn *rtmp.Conn
	state := BEGIN
	var err error
	var req models.LiveRestreamReq
	var resp models.LiveRestreamResp

loop:
	for {
		select {
		case signal := <-monitorChan:
			log.Log.Info("HandleLiveRestream: ControlSignal: " + signal)
			if strings.Compare(signal, "internal-stop") == 0 {
				log.Log.Info("HandleLiveRestream: internal-stop, Close restream")
				// close rtmp conn
				if proxyConn != nil {
					proxyConn.Close()
					proxyConn = nil
				}
				state = STOPPED
				// mqttPublish(&resp, configuration, &mqttClient)
				// stop goroutines
				break loop

			} else if strings.HasPrefix(signal, "closed") { // closed:reason-of-closed
				// CopyStream stop, error or any reason
				reason := strings.SplitN(signal, ":", 2)[1]
				log.Log.Debug("Error: " + reason)
				if proxyConn != nil {
					proxyConn.Close()
					proxyConn = nil
				}

				// check if the prev state is streamming
				if isStarted(state) {
					state = ERRORSTREAM

					response := models.LiveRestreamResp{
						ReqId:    req.ReqId,
						RespTime: time.Now().UnixMilli(),
						Stream:   req.Stream,
						Duration: time.Now().UnixMilli() - resp.RespTime,
						Status:   state.String(),
						Result:   reason,
					}

					// response to mqtt topic
					mqttPublish(&response, configuration, &mqttClient)

					// relax a moment
					time.Sleep(100 * time.Millisecond)

					// Make a retry
					retryReq := req // copy
					retryReq.ReqType = "on"
					go func() {
						timeout := time.After(5 * time.Second)
						select {
						case reqChan <- retryReq:
							log.Log.Info("HandleLiveRestream: Retry connect to proxy")
						case <-timeout:
							log.Log.Error("HandleLiveRestream: Timeout to send a retryReq")
						}
					}()
				}

			} else {
				log.Log.Error("HandleLiveRestream: unknown control signal")
			}

		case req = <-reqChan:

			log.Log.Info(fmt.Sprintf("Restream %s, current state: %s", req.Stream, state.String()))

			switch reqType := req.ReqType; strings.TrimSpace(strings.ToLower(reqType)) {
			case "on":
				log.Log.Info(fmt.Sprintf("Restream %s liveon request", req.Stream))

				// check already started
				if isStarted(state) && proxyConn != nil {
					log.Log.Info(fmt.Sprintf("Restream %s: starting or started befored!", req.Stream))
					resp.Duration = time.Now().UnixMilli() - resp.RespTime
					mqttPublish(&resp, configuration, &mqttClient)
					continue
				}

				state = STARTING
				proxyConn, err = connectWithTimeout(req, time.Second*120) // retry with timeout after 2 minutes

				if err == nil { // connect ok
					log.Log.Info("connected to proxy")
					// restreaming until close proxy connection
					// go avutil.CopyFile(proxyConn, streamCursor)
					go CopyStream(proxyConn, streamCursor, monitorChan)

					state = STARTED
					resp = models.LiveRestreamResp{
						ReqId:    req.ReqId,
						RespTime: time.Now().UnixMilli(),
						Stream:   req.Stream,
						Status:   state.String(),
						Result:   req.ProxyAddr,
					}
				} else {
					state = ERRORCONNECT
					resp = models.LiveRestreamResp{
						ReqId:    req.ReqId,
						RespTime: time.Now().UnixMilli(),
						Stream:   req.Stream,
						Status:   state.String(),
						Result:   "Can not connected: " + err.Error(),
					}
				}

				// response to mqtt topic
				mqttPublish(&resp, configuration, &mqttClient)
				// save instance
				mutex.Lock()
				StoreInstance(resp)
				mutex.Unlock()

			case "off":
				log.Log.Info(fmt.Sprintf("HandleLiveRestream: Restream %s liveoff request", req.Stream))
				if isStarted(state) && proxyConn != nil {
					log.Log.Info(fmt.Sprintf("HandleLiveRestream: Close restream %s", req.Stream))

					state = STOPPED
					url := req.ProxyAddr
					// close rtmp conn
					if proxyConn != nil {
						url = proxyConn.URL.String()
						proxyConn.Close()
						proxyConn = nil
					}

					resp = models.LiveRestreamResp{
						ReqId:    req.ReqId,
						RespTime: time.Now().UnixMilli(),
						Stream:   req.Stream,
						Status:   state.String(),
						Result:   url,
					}

					// response to mqtt topic
					mqttPublish(&resp, configuration, &mqttClient)
					// save instance
					mutex.Lock()
					StoreInstance(resp)
					mutex.Unlock()
					// Clean up, force garbage collection
					runtime.GC()
				} else {
					resp = models.LiveRestreamResp{
						ReqId:    req.ReqId,
						RespTime: time.Now().UnixMilli(),
						Stream:   req.Stream,
						Status:   NOTSTARTED.String(),
						Result:   req.ProxyAddr,
					}
					// response to mqtt topic
					mqttPublish(&resp, configuration, &mqttClient)
				}

			default:
				log.Log.Info(fmt.Sprintf("HandleLiveRestream: Unknown request for %s", req.Stream))
			}
		}
	}
}

func HandleLiveRestream(mainstreamCursor *pubsub.QueueCursor, substreamCursor *pubsub.QueueCursor,
	configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client) {

	log.Log.Info("HandleLiveRestream: starting")

	var wg sync.WaitGroup
	var mutex sync.Mutex
	mainChanReq := make(chan models.LiveRestreamReq, 1)
	mainChanMonitor := make(chan string, 1)
	subChanReq := make(chan models.LiveRestreamReq, 1)
	subChanMonitor := make(chan string, 1)
	defer close(mainChanReq)
	defer close(mainChanMonitor)
	defer close(subChanReq)
	defer close(subChanMonitor)

	wg.Add(1)
	go handle(mainChanReq, mainChanMonitor, mainstreamCursor, configuration, mqttClient, &mutex, &wg)
	if substreamCursor != nil {
		wg.Add(1)
		go handle(subChanReq, subChanMonitor, substreamCursor, configuration, mqttClient, &mutex, &wg)
	}

loop:
	for {
		select {
		// control signal
		case <-communication.LiveRestreamControl:
			log.Log.Info("Got stop signal ....")
			mainChanMonitor <- "internal-stop"
			if substreamCursor != nil {
				subChanMonitor <- "internal-stop"
			}
			// wait a moment
			time.Sleep(100 * time.Millisecond)
			break loop

		// block and wait for request
		case req := <-communication.HandleLiveRestream:
			log.Log.Info(fmt.Sprintf("HandleLiveRestream: request: %+v", req))

			switch stream := req.Stream; strings.TrimSpace(strings.ToLower(stream)) {
			case "sub", "substream":
				if substreamCursor == nil {
					log.Log.Info("SubStream is not enabled")
				} else {
					req.Stream = "SubStream"
					subChanReq <- req
				}
			default: // others is the main stream
				req.Stream = "MainStream"
				mainChanReq <- req
			}
		}
	}

	wg.Wait()

	log.Log.Info("HandleLiveRestream: finished")
}
