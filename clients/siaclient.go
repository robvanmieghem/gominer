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
	SubmitHeader(header []byte) (err error)
}

//HeaderProvider supplies headers for a miner to mine on
type HeaderProvider interface {
	//GetHeaderForWork providers a header to mine on
	GetHeaderForWork() (target, header []byte, err error)
}

// SiaClient is the Definition a client towards the sia network
type SiaClient interface {
	HeaderProvider
	HeaderReporter
	//Start connects to a sia daemon and starts supplying valid headers
	// It can be empty in case of a "getwork" implementation or maintain a tcp connection in case of stratum for example
	Start()
}

// NewSiaClient creates a new SiadClient given a '[stratum+tcp://]host:port' connectionstring
func NewSiaClient(connectionstring string) (sc SiaClient) {
	if strings.HasPrefix(connectionstring, "stratum+tcp://") {
		sc = &SiaStratumClient{connectionstring: strings.TrimPrefix(connectionstring, "stratum+tcp://")}
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

//GetHeaderForWork fetches new work from the SIA daemon
func (sc *SiadClient) GetHeaderForWork() (target, header []byte, err error) {
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
func (sc *SiadClient) SubmitHeader(header []byte) (err error) {
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
