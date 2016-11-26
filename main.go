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
	"github.com/robvanmieghem/gominer/clients"
)

//Version is the released version string of gominer
var Version = "0.5-Dev"

var intensity = 28
var devicesTypesForMining = cl.DeviceTypeGPU

const maxUint32 = int64(^uint32(0))

func createWork(siaclient clients.SiaClient, miningWorkChannel chan *MiningWork, nrOfMiningDevices int, globalItemSize int) {
	//Register a function to clear the generated work if a job gets deprecated
	// It does not matter if we clear too many, is worse to work on a stale job
	siaclient.SetDeprecatedJobCall(func() {
		numberOfWorkItemsToRemove := len(miningWorkChannel)
		for i := 0; i < numberOfWorkItemsToRemove; i++ {
			<-miningWorkChannel
		}
	})

	siaclient.Start()

	for {
		target, header, deprecationChannel, job, err := siaclient.GetHeaderForWork()

		if err != nil {
			log.Println("ERROR fetching work -", err)
			time.Sleep(1000 * time.Millisecond)
			continue
		}

		//copy target to header
		for i := 0; i < 8; i++ {
			header[i+32] = target[7-i]
		}
		//Fill the workchannel with work
		// Only generate nonces for a 32 bit space (since gpu's are mostly 32 bit)
		for i := int64(0); i*int64(globalItemSize) < (maxUint32 - int64(globalItemSize)); i++ {
			//Do not continue mining the 32 bit nonce space if the current job is deprecated
			select {
			case <-deprecationChannel:
				break
			default:
			}

			miningWorkChannel <- &MiningWork{header, int(i) * globalItemSize, job}
		}
	}
}

func main() {
	printVersion := flag.Bool("v", false, "Show version and exit")
	useCPU := flag.Bool("cpu", false, "If set, also use the CPU for mining, only GPU's are used by default")
	flag.IntVar(&intensity, "I", intensity, "Intensity")
	siadHost := flag.String("url", "localhost:9980", "siad host and port, for stratum servers, use `stratum+tcp://<host>:<port>`")
	pooluser := flag.String("user", "payoutaddress.rigname", "username, most stratum servers take this in the form [payoutaddress].[rigname]")
	excludedGPUs := flag.String("E", "", "Exclude GPU's: comma separated list of devicenumbers")
	flag.Parse()

	if *printVersion {
		fmt.Println("gominer version", Version)
		os.Exit(0)
	}

	siaclient := clients.NewSiaClient(*siadHost, *pooluser)

	if *useCPU {
		devicesTypesForMining = cl.DeviceTypeAll
	}
	globalItemSize := int(math.Exp2(float64(intensity)))

	platforms, err := cl.GetPlatforms()
	if err != nil {
		log.Panic(err)
	}

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

	//Start fetching work
	workChannel := make(chan *MiningWork, nrOfMiningDevices)
	go createWork(siaclient, workChannel, nrOfMiningDevices, globalItemSize)

	//Start mining routines
	var hashRateReportsChannel = make(chan *HashRateReport, nrOfMiningDevices*10)
	for i, device := range clDevices {
		if deviceExcludedForMining(i, *excludedGPUs) {
			continue
		}
		miner := &Miner{
			clDevice:          device,
			minerID:           i,
			hashRateReports:   hashRateReportsChannel,
			miningWorkChannel: workChannel,
			GlobalItemSize:    globalItemSize,
			siad:              siaclient,
		}
		go miner.mine()
	}

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
