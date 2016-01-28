package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
)

var getworkurl = "http://localhost:9980/miner/headerforwork"
var submitblockurl = "http://localhost:9980/miner/submitheader"

func getHeaderForWork() (target, header []byte, err error) {
	client := &http.Client{}

	req, err := http.NewRequest("GET", getworkurl, nil)
	if err != nil {
		return
	}

	req.Header.Add("User-Agent", "Sia-Agent")
	resp, err := client.Do(req)

	if err != nil {
		return
	}
	defer resp.Body.Close()
	buf := make([]byte, 113)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		return
	}
	if n < 112 {
		err = errors.New("Invalid response")
	} else {
		err = nil
	}

	target = buf[:32]
	header = buf[32:112]

	return
}

func submitHeader(header []byte) (err error) {
	req, err := http.NewRequest("POST", submitblockurl, bytes.NewReader(header))
	if err != nil {
		return
	}

	req.Header.Add("User-Agent", "Sia-Agent")

	client := &http.Client{}
	_, err = client.Do(req)

	return
}
