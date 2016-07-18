package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/robvanmieghem/go-opencl/cl"
)

//Version is the released version string of gominer
var Version = "0.5-Dev"

var intensity = 28
var devicesTypesForMining = cl.DeviceTypeGPU

func getHeader(siad *SiadClient, longpoll bool) (header []byte, err error) {
	target, header, err := siad.GetHeaderForWork(longpoll)
	if err != nil {
		log.Println("ERROR fetching work -", err)
	} else {
		//copy target to header
		for i := 0; i < 8; i++ {
			header = append(header, target[7-i])
		}
	}
	return
}

func startLongPolling(siad *SiadClient) (c chan []byte) {
	c = make(chan []byte)
	go func () {
		for {
			header, err := getHeader(siad, true)
			if err != nil {
				break
			}
			c <- header
		}
		close(c)
	} ()
	return
}

func createWork(siad *SiadClient, workChannels []chan *MiningWork, secondsOfWorkPerRequestedHeader int, globalItemSize int) {
	var timeOfLastWork time.Time
	var longChan chan []byte

	for {
		var header []byte
		var err error

		waitDuration := time.Second * time.Duration(secondsOfWorkPerRequestedHeader)
		// If long polling is enabled, request work less often
		if longChan != nil {
			waitDuration = time.Second * time.Duration(60)
		}
		if time.Since(timeOfLastWork) > waitDuration {
			header, err = getHeader(siad, false)
			if err != nil {
				time.Sleep(3 * time.Second)
				continue
			}
			log.Println("Fetched new work")
		} else {
			if siad.LongPollSupport && longChan == nil {
				log.Println("Starting long polling")
				longChan = startLongPolling(siad)
			}
			select {
			case header = <-longChan:
				if header == nil {
					longChan = nil
					continue
				}
				log.Println("Long polling pushed new work")
			case <-time.After(waitDuration - time.Since(timeOfLastWork)):
				continue
			}
		}
		timeOfLastWork = time.Now()

		// Replace any old work with the new one
		for i, c := range workChannels {
			select {
			case <-c:
			default:
			}
			c <- &MiningWork{append([]byte(nil), header...), uint64(i * globalItemSize)}
		}
	}
}

func submitSolutions(siad HeaderReporter, solutionChannel chan []byte) {
	for header := range solutionChannel {
		if err := siad.SubmitHeader(header); err != nil {
			log.Println("Error submitting solution -", err)
		}
		log.Println("Submitted header:", header)
	}
}

func main() {
	printVersion := flag.Bool("v", false, "Show version and exit")
	useCPU := flag.Bool("cpu", false, "If set, also use the CPU for mining, only GPU's are used by default")
	flag.IntVar(&intensity, "I", intensity, "Intensity")
	siadHost := flag.String("H", "localhost:9980", "siad host and port")
	secondsOfWorkPerRequestedHeader := flag.Int("S", 10, "Time between calls to siad")
	excludedGPUs := flag.String("E", "", "Exclude GPU's: comma separated list of devicenumbers")
	queryString := flag.String("Q", "", "Query string")
	flag.Parse()

	if *printVersion {
		fmt.Println("gominer version", Version)
		os.Exit(0)
	}

	siad := NewSiadClient(*siadHost, *queryString)

	if *useCPU {
		devicesTypesForMining = cl.DeviceTypeAll
	}
	globalItemSize := int(math.Exp2(float64(intensity)))

	platforms, err := cl.GetPlatforms()
	if err != nil {
		log.Panic(err)
	}

	log.Printf("gominer, experimental version 0.3.1 by SiaMining.com")

	clDevices := make([]*cl.Device, 0, 4)
	for _, platform := range platforms {
		log.Println("Platform", platform.Name())
		platormDevices, err := cl.GetDevices(platform, devicesTypesForMining)
		if err != nil {
			log.Println(err)
		}
		log.Println(len(platormDevices), "device(s) found:")
		for i, device := range platormDevices {
			log.Println(i, "-", device.Type(), "-", device.Name())
			clDevices = append(clDevices, device)
		}
	}

	nrOfMiningDevices := len(clDevices)

	if nrOfMiningDevices == 0 {
		log.Println("No suitable opencl devices found")
		os.Exit(1)
	}

	solutionChannel := make(chan []byte, nrOfMiningDevices*4)
	go submitSolutions(siad, solutionChannel)

	workChannels := make([]chan *MiningWork, 0)

	//Start mining routines
	var hashRateReportsChannel = make(chan *HashRateReport, nrOfMiningDevices*10)
	for i, device := range clDevices {
		if deviceExcludedForMining(i, *excludedGPUs) {
			continue
		}
		workChannel := make(chan *MiningWork, 1)
		workChannels = append(workChannels, workChannel)
		miner := &Miner{
			clDevice:          device,
			minerID:           i,
			minerCount:        nrOfMiningDevices,
			hashRateReports:   hashRateReportsChannel,
			miningWorkChannel: workChannel,
			solutionChannel:   solutionChannel,
			GlobalItemSize:    globalItemSize,
		}
		go miner.mine()
	}

	//Start fetching work
	go createWork(siad, workChannels, *secondsOfWorkPerRequestedHeader, globalItemSize)

	//Start printing out the hashrates of the different gpu's
	hashRateReports := make([]float64, nrOfMiningDevices)
	for {
		//No need to print at every hashreport, we have time
		for i := 0; i < nrOfMiningDevices; i++ {
			report := <-hashRateReportsChannel
			hashRateReports[report.MinerID] = report.HashRate
		}
		fmt.Print("\r")
		var totalHashRate float64
		for minerID, hashrate := range hashRateReports {
			fmt.Printf("%d-%.1f ", minerID, hashrate)
			totalHashRate += hashrate
		}
		fmt.Printf("Total: %.1f MH/s  ", totalHashRate)

	}
}

//deviceExcludedForMining checks if the device is in the exclusion list
func deviceExcludedForMining(deviceID int, excludedGPUs string) bool {
	excludedGPUList := strings.Split(excludedGPUs, ",")
	for _, excludedGPU := range excludedGPUList {
		if strconv.Itoa(deviceID) == excludedGPU {
			return true
		}
	}
	return false
}
