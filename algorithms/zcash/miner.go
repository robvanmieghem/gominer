package zcash

import (
	"fmt"
	"log"
	"time"
	"unsafe"

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

type solst struct {
	nr             uint
	likelyInvalids uint
	valid          [maxSolutions]uint8
	values         [maxSolutions][(1 << equihashParamK)]uint
}

func numberOfComputeUnits(gpu string) int {
	if gpu == "rx480" {
		return 36
	}
	log.Panicln("Unknown GPU: ", gpu)
	return 0
}

func selectWorkSizeBlake() (workSize int) {
	workSize =
		64 * /* thread per wavefront */
			blakeWPS * /* wavefront per simd */
			4 * /* simd per compute unit */
			numberOfComputeUnits("rx480")
	// Make the work group size a multiple of the nr of wavefronts, while
	// dividing the number of inputs. This results in the worksize being a
	// power of 2.
	for (numberOfInputs % workSize) != 0 {
		workSize += 64
	}
	return
}

func (miner *singleDeviceMiner) mine() {
	rowsPerUint := 4
	if numberOfSlots < 16 {
		rowsPerUint = 8
	}

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

	bufferSolutions := mining.CreateEmptyBuffer(context, cl.MemReadWrite, int(unsafe.Sizeof(solst{})))
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

		// Process first BLAKE2b-400 block
		blake := &blake2b_state_t{}
		zcash_blake2b_init(blake, zcashHashLength, equihashParamN, equihashParamK)
		zcash_blake2b_update(blake, header[:128], false)
		bufferBlake, err := context.CreateBufferUnsafe(cl.MemReadOnly|cl.MemCopyHostPtr, 64, unsafe.Pointer(&blake.h))
		if err != nil {
			log.Panic(err)
		}
		var globalWorkgroupSize int
		var localWorkgroupSize int
		for round := 0; round < equihashParamK; round++ {
			// Now on every round!
			localWorkgroupSize = 256
			globalWorkgroupSize = numberOfRows / rowsPerUint
			kernelInitHt.SetArgBuffer(0, bufferHt[round%2])
			kernelInitHt.SetArgBuffer(1, bufferRowCounters[round%2])
			commandQueue.EnqueueNDRangeKernel(kernelInitHt, nil, []int{globalWorkgroupSize}, []int{localWorkgroupSize}, nil)

			if round == 0 {
				kernelRounds[round].SetArgBuffer(0, bufferBlake)
				kernelRounds[round].SetArgBuffer(1, bufferHt[round%2])
				kernelRounds[round].SetArgBuffer(2, bufferRowCounters[round%2])
				globalWorkgroupSize = selectWorkSizeBlake()
				kernelRounds[round].SetArgBuffer(3, bufferDbg)
			} else {
				kernelRounds[round].SetArgBuffer(0, bufferHt[(round-1)%2])
				kernelRounds[round].SetArgBuffer(1, bufferHt[round%2])
				kernelRounds[round].SetArgBuffer(2, bufferRowCounters[(round-1)%2])
				kernelRounds[round].SetArgBuffer(3, bufferRowCounters[round%2])
				globalWorkgroupSize = numberOfRows
				kernelRounds[round].SetArgBuffer(4, bufferDbg)
			}
			if round == equihashParamK-1 {
				kernelRounds[round].SetArgBuffer(5, bufferSolutions)
			}
			localWorkgroupSize = 64
			commandQueue.EnqueueNDRangeKernel(kernelRounds[round], nil, []int{globalWorkgroupSize}, []int{localWorkgroupSize}, nil)
		}
		kernelSolutions.SetArgBuffer(0, bufferHt[0])
		kernelSolutions.SetArgBuffer(1, bufferHt[1])
		kernelSolutions.SetArgBuffer(2, bufferSolutions)
		kernelSolutions.SetArgBuffer(3, bufferRowCounters[0])
		kernelSolutions.SetArgBuffer(4, bufferRowCounters[1])
		globalWorkgroupSize = numberOfRows
		commandQueue.EnqueueNDRangeKernel(kernelSolutions, nil, []int{globalWorkgroupSize}, []int{localWorkgroupSize}, nil)
		// read solutions
		solutionsFound := miner.verifySolutions(commandQueue, bufferSolutions, header, target, job)
		bufferBlake.Release()
		log.Println("Solutions found:", solutionsFound)

		hashRate := float64(solutionsFound) / (time.Since(start).Seconds() * 1000000)
		miner.HashRateReports <- &mining.HashRateReport{MinerID: miner.MinerID, HashRate: hashRate}
	}

}

func (miner *singleDeviceMiner) verifySolutions(commandQueue *cl.CommandQueue, bufferSolutions *cl.MemObject, header []byte, target []byte, job interface{}) (solutionsFound int) {

	sols := &solst{}

	// Most OpenCL implementations of clEnqueueReadBuffer in blocking mode are
	// good, except Nvidia implementing it as a wasteful busywait.
	commandQueue.EnqueueReadBuffer(bufferSolutions, true, 0, int(unsafe.Sizeof(*sols)), unsafe.Pointer(sols), nil)

	// let's check these solutions we just read...
	if sols.nr > maxSolutions {
		log.Printf("ERROR: %d (probably invalid) solutions were dropped!\n", sols.nr-maxSolutions)
		sols.nr = maxSolutions
	}
	for i := 0; uint(i) < sols.nr; i++ {
		solutionsFound += miner.verifySolution(sols, i)
	}
	miner.submitSolution(sols, solutionsFound, header, target, job)

	return
}

func (miner *singleDeviceMiner) verifySolution(sols *solst, index int) int {
	inputs := sols.values[index]
	seenLength := (1 << (prefix + 1)) / 8
	seen := make([]uint8, seenLength, seenLength)
	var i uint32
	var tmp uint8
	// look for duplicate inputs
	for i = 0; i < (1 << equihashParamK); i++ {
		if inputs[i]/uint(8) >= uint(seenLength) {
			log.Printf("Invalid input retrieved from device: %d\n", inputs[i])
			sols.valid[index] = 0
			return 0
		}
		tmp = seen[inputs[i]/8]
		seen[inputs[i]/8] |= 1 << (inputs[i] & 7)
		if tmp == seen[inputs[i]/8] {
			// at least one input value is a duplicate
			sols.valid[index] = 0
			return 0
		}
	}
	// the valid flag is already set by the GPU, but set it again because
	// I plan to change the GPU code to not set it
	sols.valid[index] = 1
	// sort the pairs in place
	for level := 0; level < equihashParamK; level++ {
		for i = 0; i < (1 << equihashParamK); i += (2 << uint(level)) {
			sortPair(&inputs[i], 1<<uint(level))
		}
	}
	return 1
}

func (miner *singleDeviceMiner) submitSolution(solutions *solst, solutionsFound int, header []byte, target []byte, job interface{}) {
	//TODO
}

func sortPair(inputs *uint, level int) {
	//TODO
}
