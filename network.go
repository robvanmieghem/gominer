package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
)

// SiadClient is used to connect to siad
type SiadClient struct {
	siadurl string
}

// NewSiadClient creates a new SiadClient given a 'host:port' connectionstring
func NewSiadClient(connectionstring string) *SiadClient {
	s := SiadClient{}
	s.siadurl = "http://" + connectionstring + "/miner/header"
	return &s
}

func (sc *SiadClient) getHeaderForWork() (target, header []byte, err error) {
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
		err = fmt.Errorf("Invalid siad response, status code %d, is your wallet initialized and unlocked?", resp.StatusCode)
		return
	default:
		err = fmt.Errorf("Invalid siad, status code %d", resp.StatusCode)
		return
	}
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	if len(buf) < 112 {
		err = fmt.Errorf("Invalid siad response, only received %d bytes, is your wallet initialized and unlocked?", len(buf))
		return
	}

	target = buf[:32]
	header = buf[32:112]

	return
}

func (sc *SiadClient) submitHeader(header []byte) (err error) {
	req, err := http.NewRequest("POST", sc.siadurl, bytes.NewReader(header))
	if err != nil {
		return
	}

	req.Header.Add("User-Agent", "Sia-Agent")

	client := &http.Client{}
	_, err = client.Do(req)

	return
}
