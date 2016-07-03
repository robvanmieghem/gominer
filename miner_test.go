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
	offset          uint64
	submittedHeader []byte
	intensity       int
}{
	{
		height:          56206,
		hash:            "00000000000006418b86014ff54b457f52665b428d5af57e80b0b7ec84c706e5",
		workHeader:      []byte{0, 0, 0, 0, 0, 0, 26, 158, 25, 209, 169, 53, 113, 22, 90, 11, 72, 7, 222, 103, 247, 244, 163, 156, 158, 5, 53, 126, 186, 215, 88, 48, 45, 32, 0, 0, 0, 0, 0, 0, 20, 25, 103, 87, 0, 0, 0, 0, 218, 189, 84, 137, 247, 169, 197, 113, 213, 120, 125, 148, 92, 197, 47, 212, 250, 153, 114, 53, 199, 209, 183, 97, 28, 242, 206, 120, 191, 202, 34, 9, 45, 32, 0, 0, 0, 0, 0, 0},
		offset:          5 * uint64(math.Exp2(float64(28))),
		submittedHeader: []byte{0, 0, 0, 0, 0, 0, 26, 158, 25, 209, 169, 53, 113, 22, 90, 11, 72, 7, 222, 103, 247, 244, 163, 156, 158, 5, 53, 126, 186, 215, 88, 48, 88, 47, 107, 95, 0, 0, 0, 0, 20, 25, 103, 87, 0, 0, 0, 0, 218, 189, 84, 137, 247, 169, 197, 113, 213, 120, 125, 148, 92, 197, 47, 212, 250, 153, 114, 53, 199, 209, 183, 97, 28, 242, 206, 120, 191, 202, 34, 9},
		intensity:       28,
	},
	{
		height:          57653,
		hash:            "00000000000001ccac64b49a9ebc69c6046a93f4d32d8f8f6967c8f487ed8cec",
		workHeader:      []byte{0, 0, 0, 0, 0, 0, 6, 72, 174, 217, 105, 206, 174, 59, 150, 117, 251, 55, 209, 192, 241, 37, 35, 184, 2, 194, 253, 173, 207, 249, 114, 1, 62, 26, 0, 0, 0, 0, 0, 0, 41, 7, 115, 87, 0, 0, 0, 0, 56, 56, 181, 217, 76, 24, 251, 231, 137, 4, 166, 20, 40, 53, 77, 36, 148, 23, 138, 146, 2, 199, 168, 122, 71, 162, 44, 150, 144, 2, 198, 67, 62, 26, 0, 0, 0, 0, 0, 0},
		offset:          805306368,
		submittedHeader: []byte{0, 0, 0, 0, 0, 0, 6, 72, 174, 217, 105, 206, 174, 59, 150, 117, 251, 55, 209, 192, 241, 37, 35, 184, 2, 194, 253, 173, 207, 249, 114, 1, 7, 235, 26, 63, 0, 0, 0, 0, 41, 7, 115, 87, 0, 0, 0, 0, 56, 56, 181, 217, 76, 24, 251, 231, 137, 4, 166, 20, 40, 53, 77, 36, 148, 23, 138, 146, 2, 199, 168, 122, 71, 162, 44, 150, 144, 2, 198, 67},
		intensity:       28,
	},
}

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
		minerCount:        1,
		hashRateReports:   hashRateReportsChannel,
		GlobalItemSize:    int(math.Exp2(float64(28))),
		miningWorkChannel: workChannel,
		solutionChannel:   validator.submittedHeaders,
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
