package main

import (
	"io/ioutil"
	"log"
	"time"

	"./cl"
)

func min(a, b int) int {
	if a > b {
		return b
	}
	return a
}

//HashRateReport is sent from the mining routines for giving combined information as output
type HashRateReport struct {
	MinerID  int
	HashRate float64
}

//MiningWork is sent to the mining routines and defines what ranges should be searched for a matching nonce
type MiningWork struct {
	Header []byte
	Offset uint64
}

// Miner actually mines :-)
type Miner struct {
	clDevice          *cl.Device
	minerID           int
	hashRateReports   chan *HashRateReport
	miningWorkChannel chan *MiningWork
	GlobalItemSize    int
	siad              HeaderReporter
}

func (miner *Miner) mine() {
	log.Println(miner.minerID, "- Initializing", miner.clDevice.Type(), "-", miner.clDevice.Name())

	context, err := cl.CreateContext([]*cl.Device{miner.clDevice})
	if err != nil {
		log.Fatalln(miner.minerID, "-", err)
	}
	defer context.Release()

	commandQueue, err := context.CreateCommandQueue(miner.clDevice, 0)
	if err != nil {
		log.Fatalln(miner.minerID, "-", err)
	}
	defer commandQueue.Release()

	ksrc, lerr := ioutil.ReadFile("./kernel-simd_x64.spirv")
	if lerr != nil {
		log.Fatalln("Problems with loading file... ", lerr)
	}

	program, err := context.CreateProgramWithIL(ksrc)
	if err != nil {
		log.Fatalln(miner.minerID, "-", err)
	}
	defer program.Release()

	err = program.BuildProgram([]*cl.Device{miner.clDevice}, "-cl-unsafe-math-optimizations -cl-mad-enable")
	if err != nil {
		log.Fatalln(miner.minerID, "-", err)
	}

	kernel, err := program.CreateKernel("nonceGrind")
	if err != nil {
		log.Fatalln(miner.minerID, "-", err)
	}
	defer kernel.Release()

	blockHeaderObj, err := context.CreateEmptyBuffer(cl.MemReadOnly, 88)
	if err != nil {
		log.Fatalln(miner.minerID, "-", err)
	}
	defer blockHeaderObj.Release()
	kernel.SetArgBuffer(0, blockHeaderObj)

	nonceOut := make([]byte, 8, 8)
	nonceOutObj, err := context.CreateBuffer(cl.MemReadWrite|cl.MemUseHostPtr, nonceOut)
	if err != nil {
		log.Fatalln(miner.minerID, "-", err)
	}
	defer nonceOutObj.Release()
	kernel.SetArgBuffer(1, nonceOutObj)

	localItemSize, err := kernel.WorkGroupSize(miner.clDevice)
	//localItemSize = min(localItemSize, 256)
	if err != nil {
		log.Fatalln(miner.minerID, "- WorkGroupSize failed -", err)
	}

	log.Println(miner.minerID, "- Global item size:", miner.GlobalItemSize, "(Intensity", intensity, ")", "- Local item size:", localItemSize)

	log.Println(miner.minerID, "- Initialized ", miner.clDevice.Type(), "-", miner.clDevice.Name())

	if _, err = commandQueue.EnqueueWriteBufferByte(nonceOutObj, true, 0, nonceOut, nil); err != nil {
		log.Fatalln(miner.minerID, "-", err)
	}
	for {
		start := time.Now()
		var work *MiningWork
		continueMining := true
		select {
		case work, continueMining = <-miner.miningWorkChannel:
		default:
			log.Println(miner.minerID, "-", "No work ready")
			work, continueMining = <-miner.miningWorkChannel
			log.Println(miner.minerID, "-", "Continuing")
		}
		if !continueMining {
			log.Println("Halting miner ", miner.minerID)
			break
		}
		//Copy high 32 bits of Offset to Header, clear low 32 bits
		for i := 0; i < 4; i++ {
			work.Header[i+32] = 0
		}
		for i := 4; i < 8; i++ {
			work.Header[i+32] = byte(work.Offset >> uint(i*8))
		}
		//Copy input to kernel args
		if _, err = commandQueue.EnqueueWriteBufferByte(blockHeaderObj, true, 0, work.Header, nil); err != nil {
			log.Fatalln(miner.minerID, "-", err)
		}

		//Run the kernel
		if _, err = commandQueue.EnqueueNDRangeKernel(kernel, []int{int(work.Offset)}, []int{miner.GlobalItemSize / 4}, []int{localItemSize}, nil); err != nil {
			log.Fatalln(miner.minerID, "-", err)
		}
		commandQueue.Finish()

		//Get output
		/*if _, err = commandQueue.EnqueueReadBufferByte(nonceOutObj, true, 0, nonceOut, nil); err != nil {
			log.Fatalln(miner.minerID, "-", err)
		}*/
		//Check if match found
		if nonceOut[0] != 0 || nonceOut[1] != 0 || nonceOut[2] != 0 || nonceOut[3] != 0 || nonceOut[4] != 0 || nonceOut[5] != 0 || nonceOut[6] != 0 || nonceOut[7] != 0 {
			log.Println(miner.minerID, "-", "Yay, solution found!")
			if nonceOut[0] == 0 {
				log.Println(miner.minerID, "-", "Solution found with a nonce that started with 0...")
			}
			// Copy nonce to a new header.
			header := append([]byte(nil), work.Header[:80]...)
			for i := 0; i < 8; i++ {
				header[i+32] = nonceOut[i]
			}
			go func() {
				if err := miner.siad.SubmitHeader(header); err != nil {
					log.Println(miner.minerID, "- Error submitting solution -", err)
				}
			}()
			log.Println("Work header:", work.Header)
			log.Println("Offset:", work.Offset)
			log.Println("Submitted header:", header)

			//Clear the output since it is dirty now
			for i := 0; i < 8; i++ {
				nonceOut[i] = 0
			}
			/*if _, err = commandQueue.EnqueueWriteBufferByte(nonceOutObj, true, 0, nonceOut, nil); err != nil {
				log.Fatalln(miner.minerID, "-", err)
			}*/
		}

		hashRate := float64(miner.GlobalItemSize) / (time.Since(start).Seconds() * 1000000)
		miner.hashRateReports <- &HashRateReport{miner.minerID, hashRate}
	}

}
