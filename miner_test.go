package main

import (
	"bytes"
	"log"
	"math"
	"testing"

	"github.com/robvanmieghem/go-opencl/cl"
)

var provenSolutions = []struct {
	height          int
	hash            string
	workHeader      []byte
	offset          int
	submittedHeader []byte
	intensity       int
}{{
	height:          56206,
	hash:            "00000000000006418b86014ff54b457f52665b428d5af57e80b0b7ec84c706e5",
	workHeader:      []byte{0, 0, 0, 0, 0, 0, 26, 158, 25, 209, 169, 53, 113, 22, 90, 11, 72, 7, 222, 103, 247, 244, 163, 156, 158, 5, 53, 126, 186, 215, 88, 48, 45, 32, 0, 0, 0, 0, 0, 0, 20, 25, 103, 87, 0, 0, 0, 0, 218, 189, 84, 137, 247, 169, 197, 113, 213, 120, 125, 148, 92, 197, 47, 212, 250, 153, 114, 53, 199, 209, 183, 97, 28, 242, 206, 120, 191, 202, 34, 9},
	offset:          5 * int(math.Exp2(float64(28))),
	submittedHeader: []byte{0, 0, 0, 0, 0, 0, 26, 158, 25, 209, 169, 53, 113, 22, 90, 11, 72, 7, 222, 103, 247, 244, 163, 156, 158, 5, 53, 126, 186, 215, 88, 48, 88, 47, 107, 95, 0, 0, 0, 0, 20, 25, 103, 87, 0, 0, 0, 0, 218, 189, 84, 137, 247, 169, 197, 113, 213, 120, 125, 148, 92, 197, 47, 212, 250, 153, 114, 53, 199, 209, 183, 97, 28, 242, 206, 120, 191, 202, 34, 9},
	intensity:       28,
}}

func TestMine(t *testing.T) {
	platforms, err := cl.GetPlatforms()
	if err != nil {
		log.Panic(err)
	}

	var clDevice *cl.Device
	for _, platform := range platforms {
		platormDevices, err := cl.GetDevices(platform, devicesTypesForMining)
		if err != nil {
			log.Fatalln(err)
		}
		for _, device := range platormDevices {
			log.Println(device.Type(), "-", device.Name())
			clDevice = device
		}
	}

	workChannel := make(chan *MiningWork, len(provenSolutions)+1)

	for _, provenSolution := range provenSolutions {
		workChannel <- &MiningWork{provenSolution.workHeader, provenSolution.offset}
	}
	close(workChannel)
	var hashRateReportsChannel = make(chan *HashRateReport, len(provenSolutions)+1)
	validator := newSubmittedHeaderValidator(len(provenSolutions))
	miner := &Miner{
		clDevice:          clDevice,
		minerID:           0,
		hashRateReports:   hashRateReportsChannel,
		GlobalItemSize:    int(math.Exp2(float64(28))),
		miningWorkChannel: workChannel,
		siad:              validator,
	}
	miner.mine()
	validator.validate(t)
}

func newSubmittedHeaderValidator(capacity int) (v *submittedHeaderValidator) {
	v = &submittedHeaderValidator{}
	v.submittedHeaders = make(chan []byte, capacity)
	return
}

type submittedHeaderValidator struct {
	submittedHeaders chan []byte
}

//SubmitHeader stores solved so they can later be validated after the testrun
func (v *submittedHeaderValidator) SubmitHeader(header []byte) (err error) {
	v.submittedHeaders <- header
	return
}

func (v *submittedHeaderValidator) validate(t *testing.T) {
	if len(v.submittedHeaders) != len(provenSolutions) {
		t.Fatal("Wrong number of headers reported")
	}
	for _, provenSolution := range provenSolutions {
		submittedHeader := <-v.submittedHeaders
		if !bytes.Equal(submittedHeader, provenSolution.submittedHeader) {
			t.Error("Mismatch\nExpected header: ", provenSolution.submittedHeader, "\nSubmitted header: ", submittedHeader)
		}
	}
}
