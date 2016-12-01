package zcash

import (
	"fmt"
	"log"
	"time"

	"github.com/robvanmieghem/go-opencl/cl"
	"github.com/robvanmieghem/gominer/clients"
	"github.com/robvanmieghem/gominer/mining"
)

// Miner actually mines :-)
type Miner struct {
	ClDevices       map[int]*cl.Device
	HashRateReports chan *mining.HashRateReport
	Client          clients.Client
}

//singleDeviceMiner actually mines on 1 opencl device
type singleDeviceMiner struct {
	ClDevice        *cl.Device
	MinerID         int
	HashRateReports chan *mining.HashRateReport
	Client          clients.Client
}

//Mine spawns a seperate miner for each device defined in the CLDevices and feeds it with work
func (m *Miner) Mine() {

	m.Client.Start()
	for minerID, device := range m.ClDevices {
		sdm := &singleDeviceMiner{
			ClDevice:        device,
			MinerID:         minerID,
			HashRateReports: m.HashRateReports,
			Client:          m.Client,
		}
		go sdm.mine()

	}
}

func (miner *singleDeviceMiner) mine() {
	log.Println(miner.MinerID, "- Initializing", miner.ClDevice.Type(), "-", miner.ClDevice.Name())

	context, err := cl.CreateContext([]*cl.Device{miner.ClDevice})
	if err != nil {
		log.Fatalln(miner.MinerID, "-", err)
	}
	defer context.Release()

	commandQueue, err := context.CreateCommandQueue(miner.ClDevice, 0)
	if err != nil {
		log.Fatalln(miner.MinerID, "-", err)
	}
	defer commandQueue.Release()

	program, err := context.CreateProgramWithSource([]string{kernelSource})
	if err != nil {
		log.Fatalln(miner.MinerID, "-", err)
	}
	defer program.Release()

	err = program.BuildProgram([]*cl.Device{miner.ClDevice}, "")
	if err != nil {
		log.Fatalln(miner.MinerID, "-", err)
	}

	//Create kernels
	kernelInitHt, err := program.CreateKernel("kernel_init_ht")
	if err != nil {
		log.Fatalln(miner.MinerID, "-", err)
	}
	defer kernelInitHt.Release()

	var kernelRounds [equihashParamK]*cl.Kernel
	for round := 0; round < equihashParamK; round++ {
		kernelRounds[round], err = program.CreateKernel(fmt.Sprintf("kernel_round%d", round))
		if err != nil {
			log.Fatalln(miner.MinerID, "-", err)
		}
		defer kernelRounds[round].Release()
	}

	kernelSolutions, err := program.CreateKernel("kernel_sols")
	if err != nil {
		log.Fatalln(miner.MinerID, "-", err)
	}
	defer kernelSolutions.Release()

	//Create memory buffers
	bufferDbg := mining.CreateEmptyBuffer(context, cl.MemCopyHostPtr, 8)
	defer bufferDbg.Release()

	var bufferHt [2]*cl.MemObject
	bufferHt[0] = mining.CreateEmptyBuffer(context, cl.MemReadWrite, htSize)
	defer bufferHt[0].Release()
	bufferHt[1] = mining.CreateEmptyBuffer(context, cl.MemReadWrite, htSize)
	defer bufferHt[1].Release()

	bufferSolutions := mining.CreateEmptyBuffer(context, cl.MemReadWrite, 25)
	defer bufferSolutions.Release()

	var bufferRowCounters [2]*cl.MemObject
	bufferRowCounters[0] = mining.CreateEmptyBuffer(context, cl.MemReadWrite, numberOfRows)
	defer bufferRowCounters[0].Release()
	bufferRowCounters[1] = mining.CreateEmptyBuffer(context, cl.MemReadWrite, numberOfRows)
	defer bufferRowCounters[1].Release()

	for {
		start := time.Now()
		target, header, deprecationChannel, job, err := miner.Client.GetHeaderForWork()
		if err != nil {
			log.Println("ERROR fetching work -", err)
			time.Sleep(1000 * time.Millisecond)
			continue
		}
		continueMining := true
		if !continueMining {
			log.Println("Halting miner ", miner.MinerID)
			break
		}

		log.Println(target, header, deprecationChannel, job)
		//TODO: execute the job

		//TODO: fix the calculation
		hashRate := float64(0) / (time.Since(start).Seconds() * 1000000)
		miner.HashRateReports <- &mining.HashRateReport{MinerID: miner.MinerID, HashRate: hashRate}
	}

}
