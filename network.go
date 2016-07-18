package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

//HeaderReporter defines the required method a SIA client or pool client should implement for miners to be able to report solved headers
type HeaderReporter interface {
	//SubmitHeader reports a solved header
	SubmitHeader(header []byte) (err error)
}

// SiadClient is used to connect to siad
type SiadClient struct {
	siadurl string
	LongPollSupport bool
}

// NewSiadClient creates a new SiadClient given a 'host:port' connectionstring
func NewSiadClient(connectionstring string, querystring string) *SiadClient {
	s := SiadClient{}
	s.siadurl = "http://" + connectionstring + "/miner/header?" + querystring
	return &s
}

func decodeMessage(resp *http.Response) (msg string, err error) {
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	var data struct{Message string `json:"message"`}
	if err = json.Unmarshal(buf, &data); err == nil {
		msg = data.Message
	}
	return
}

//GetHeaderForWork fetches new work from the SIA daemon
func (sc *SiadClient) GetHeaderForWork(longpoll bool) (target, header []byte, err error) {
	timeout := time.Second * 10
	if longpoll {
		timeout = time.Minute * 60
	}
	client := &http.Client{
		Timeout: timeout,
	}

	url := sc.siadurl
	if longpoll {
		url += "&longpoll"
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}

	req.Header.Add("User-Agent", "Sia-Agent")
	resp, err := client.Do(req)
	if err != nil {
		sc.LongPollSupport = false
		return
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 400:
		msg, errd := decodeMessage(resp)
		if errd != nil {
			err = fmt.Errorf("status code %d, no message", resp.StatusCode)
		} else {
			err = fmt.Errorf("status code %d, message: %s", resp.StatusCode, msg)
		}
		return
	default:
		err = fmt.Errorf("status code %d", resp.StatusCode)
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

	xme := resp.Header.Get("X-Mining-Extensions")
	if strings.Contains(xme, "longpoll") {
		sc.LongPollSupport = true
	}

	return
}

//SubmitHeader reports a solved header to the SIA daemon
func (sc *SiadClient) SubmitHeader(header []byte) (err error) {
	req, err := http.NewRequest("POST", sc.siadurl, bytes.NewReader(header))
	if err != nil {
		return
	}

	req.Header.Add("User-Agent", "Sia-Agent")

	client := &http.Client{
		Timeout: time.Second * 10,
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	switch resp.StatusCode {
	case 204:
	default:
		msg, errd := decodeMessage(resp)
		if errd != nil {
			err = fmt.Errorf("status code %d", resp.StatusCode)
		} else {
			err = fmt.Errorf("%s", msg)
		}
		return
	}
	return
}
