package main

import (
	"log"
	"time"

	"github.com/robvanmieghem/go-opencl/cl"
)

//HashRateReport is sent to from the mining routines for giving combined information as output
type HashRateReport struct {
	MinerID  int
	HashRate float64
}

//MiningWork is sent to the mining routines and defines what ranges should be searched for a matching nonce
type MiningWork struct {
	Header []byte
	Offset int
}

func mine(clDevice *cl.Device, minerID int, hashRateReports chan *HashRateReport, miningWorkChannel chan *MiningWork) {
	log.Println(minerID, "- Initializing", clDevice.Type(), "-", clDevice.Name())

	context, err := cl.CreateContext([]*cl.Device{clDevice})
	if err != nil {
		log.Fatalln(minerID, "-", err)
	}
	defer context.Release()

	commandQueue, err := context.CreateCommandQueue(clDevice, 0)
	if err != nil {
		log.Fatalln(minerID, "-", err)
	}
	defer commandQueue.Release()

	program, err := context.CreateProgramWithSource([]string{kernelSource})
	if err != nil {
		log.Fatalln(minerID, "-", err)
	}
	defer program.Release()

	err = program.BuildProgram([]*cl.Device{clDevice}, "")
	if err != nil {
		log.Fatalln(minerID, "-", err)
	}

	kernel, err := program.CreateKernel("nonceGrind")
	if err != nil {
		log.Fatalln(minerID, "-", err)
	}
	defer kernel.Release()

	blockHeaderObj, err := context.CreateEmptyBuffer(cl.MemReadOnly, 80)
	if err != nil {
		log.Fatalln(minerID, "-", err)
	}
	defer blockHeaderObj.Release()
	kernel.SetArgBuffer(0, blockHeaderObj)

	nonceOutObj, err := context.CreateEmptyBuffer(cl.MemReadWrite, 8)
	if err != nil {
		log.Fatalln(minerID, "-", err)
	}
	defer nonceOutObj.Release()
	kernel.SetArgBuffer(1, nonceOutObj)

	localItemSize, err := kernel.WorkGroupSize(clDevice)
	if err != nil {
		log.Fatalln(minerID, "- WorkGroupSize failed -", err)
	}

	log.Println(minerID, "- Global item size:", globalItemSize, "(Intensity", intensity, ")", "- Local item size:", localItemSize)

	log.Println(minerID, "- Started mining on", clDevice.Type(), "-", clDevice.Name())

	for {
		start := time.Now()

		work := <-miningWorkChannel

		//Copy input to kernel args
		if _, err = commandQueue.EnqueueWriteBufferByte(blockHeaderObj, true, 0, work.Header, nil); err != nil {
			log.Fatalln(minerID, "-", err)
		}

		nonceOut := make([]byte, 8, 8) //TODO: get this out of the for loop
		if _, err = commandQueue.EnqueueWriteBufferByte(nonceOutObj, true, 0, nonceOut, nil); err != nil {
			log.Fatalln(minerID, "-", err)
		}

		//Run the kernel
		if _, err = commandQueue.EnqueueNDRangeKernel(kernel, []int{work.Offset}, []int{globalItemSize}, []int{localItemSize}, nil); err != nil {
			log.Fatalln(minerID, "-", err)
		}
		//Get output
		if _, err = commandQueue.EnqueueReadBufferByte(nonceOutObj, true, 0, nonceOut, nil); err != nil {
			log.Fatalln(minerID, "_", err)
		}
		//Check if match found
		if nonceOut[0] != 0 {
			log.Println(minerID, "-", "Yay, block found!")
			// Copy nonce to a new header.
			header := append([]byte(nil), work.Header...)
			for i := 0; i < 8; i++ {
				header[i+32] = nonceOut[i]
			}
			if err = submitHeader(header); err != nil {
				log.Println(minerID, "- Error submitting block -", err)
			}
		}

		hashRate := float64(globalItemSize) / (time.Since(start).Seconds() * 1000000)
		hashRateReports <- &HashRateReport{minerID, hashRate}
	}

}
