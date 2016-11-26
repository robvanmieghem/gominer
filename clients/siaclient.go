package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

//HeaderReporter defines the required method a SIA client or pool client should implement for miners to be able to report solved headers
type HeaderReporter interface {
	//SubmitHeader reports a solved header
	SubmitHeader(header []byte, job interface{}) (err error)
}

//HeaderProvider supplies headers for a miner to mine on
type HeaderProvider interface {
	//GetHeaderForWork providers a header to mine on
	// the deprecationChannel is closed when the job should be abandoned
	GetHeaderForWork() (target, header []byte, deprecationChannel chan bool, job interface{}, err error)
}

//DeprecatedJobCall is a function that can be registered on a client to be executed when
// the server indicates that all previous jobs should be abandoned
type DeprecatedJobCall func()

// SiaClient is the Definition a client towards the sia network
type SiaClient interface {
	HeaderProvider
	HeaderReporter
	//Start connects to a sia daemon and starts supplying valid headers
	// It can be empty in case of a "getwork" implementation or maintain a tcp connection in case of stratum for example
	Start()
	//SetDeprecatedJobCall sets the function to be called when the previous jobs should be abandoned
	SetDeprecatedJobCall(call DeprecatedJobCall)
}

// NewSiaClient creates a new SiadClient given a '[stratum+tcp://]host:port' connectionstring
func NewSiaClient(connectionstring, pooluser string) (sc SiaClient) {
	if strings.HasPrefix(connectionstring, "stratum+tcp://") {
		sc = &SiaStratumClient{connectionstring: strings.TrimPrefix(connectionstring, "stratum+tcp://"), User: pooluser}
	} else {
		s := SiadClient{}
		s.siadurl = "http://" + connectionstring + "/miner/header"
		sc = &s
	}
	return
}

// SiadClient is a simple client to a siad
type SiadClient struct {
	siadurl string
}

func decodeMessage(resp *http.Response) (msg string, err error) {
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	var data struct {
		Message string `json:"message"`
	}
	if err = json.Unmarshal(buf, &data); err == nil {
		msg = data.Message
	}
	return
}

//Start does nothing
func (sc *SiadClient) Start() {}

//SetDeprecatedJobCall does nothing
func (sc *SiadClient) SetDeprecatedJobCall(call DeprecatedJobCall) {}

//GetHeaderForWork fetches new work from the SIA daemon
func (sc *SiadClient) GetHeaderForWork() (target []byte, header []byte, deprecationChannel chan bool, job interface{}, err error) {
	//the deprecationChannel is not used but return a valid channel anyway
	deprecationChannel = make(chan bool)

	client := &http.Client{}

	req, err := http.NewRequest("GET", sc.siadurl, nil)
	if err != nil {
		return
	}

	req.Header.Add("User-Agent", "Sia-Agent")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 400:
		msg, errd := decodeMessage(resp)
		if errd != nil {
			err = fmt.Errorf("Status code %d", resp.StatusCode)
		} else {
			err = fmt.Errorf("Status code %d, message: %s", resp.StatusCode, msg)
		}
		return
	default:
		err = fmt.Errorf("Status code %d", resp.StatusCode)
		return
	}
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	if len(buf) < 112 {
		err = fmt.Errorf("Invalid response, only received %d bytes", len(buf))
		return
	}

	target = buf[:32]
	header = buf[32:112]

	return
}

//SubmitHeader reports a solved header to the SIA daemon
func (sc *SiadClient) SubmitHeader(header []byte, job interface{}) (err error) {
	req, err := http.NewRequest("POST", sc.siadurl, bytes.NewReader(header))
	if err != nil {
		return
	}

	req.Header.Add("User-Agent", "Sia-Agent")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	switch resp.StatusCode {
	case 204:
	default:
		msg, errd := decodeMessage(resp)
		if errd != nil {
			err = fmt.Errorf("Status code %d", resp.StatusCode)
		} else {
			err = fmt.Errorf("%s", msg)
		}
		return
	}
	return
}
