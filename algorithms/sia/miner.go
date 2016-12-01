package sia

import (
	"log"
	"time"

	"github.com/robvanmieghem/go-opencl/cl"
	"github.com/robvanmieghem/gominer/clients"
	"github.com/robvanmieghem/gominer/mining"
)

//miningWork is sent to the mining routines and defines what ranges should be searched for a matching nonce
type miningWork struct {
	Header []byte
	Offset int
	Job    interface{}
}

// Miner actually mines :-)
type Miner struct {
	ClDevices         map[int]*cl.Device
	HashRateReports   chan *mining.HashRateReport
	miningWorkChannel chan *miningWork
	//Intensity defines the GlobalItemSize in a human friendly way, the GlobalItemSize = 2^Intensity
	Intensity      int
	GlobalItemSize int
	Client         clients.Client
}

//singleDeviceMiner actually mines on 1 opencl device
type singleDeviceMiner struct {
	ClDevice          *cl.Device
	MinerID           int
	HashRateReports   chan *mining.HashRateReport
	miningWorkChannel chan *miningWork
	//Intensity defines the GlobalItemSize in a human friendly way, the GlobalItemSize = 2^Intensity
	Intensity      int
	GlobalItemSize int
	Client         clients.HeaderReporter
}

//Mine spawns a seperate miner for each device defined in the CLDevices and feeds it with work
func (m *Miner) Mine() {

	m.miningWorkChannel = make(chan *miningWork, len(m.ClDevices))
	go m.createWork()
	for minerID, device := range m.ClDevices {
		sdm := &singleDeviceMiner{
			ClDevice:          device,
			MinerID:           minerID,
			HashRateReports:   m.HashRateReports,
			miningWorkChannel: m.miningWorkChannel,
			GlobalItemSize:    m.GlobalItemSize,
			Client:            m.Client,
		}
		go sdm.mine()

	}
}

const maxUint32 = int64(^uint32(0))

func (m *Miner) createWork() {
	//Register a function to clear the generated work if a job gets deprecated.
	// It does not matter if we clear too many, it is worse to work on a stale job.
	m.Client.SetDeprecatedJobCall(func() {
		numberOfWorkItemsToRemove := len(m.miningWorkChannel)
		for i := 0; i <= numberOfWorkItemsToRemove; i++ {
			<-m.miningWorkChannel
		}
	})

	m.Client.Start()

	for {
		target, header, deprecationChannel, job, err := m.Client.GetHeaderForWork()

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
	nonce32loop:
		for i := int64(0); i*int64(m.GlobalItemSize) < (maxUint32 - int64(m.GlobalItemSize)); i++ {
			//Do not continue mining the 32 bit nonce space if the current job is deprecated
			select {
			case <-deprecationChannel:
				break nonce32loop
			default:
			}

			m.miningWorkChannel <- &miningWork{header, int(i) * m.GlobalItemSize, job}
		}
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

	kernel, err := program.CreateKernel("nonceGrind")
	if err != nil {
		log.Fatalln(miner.MinerID, "-", err)
	}
	defer kernel.Release()

	blockHeaderObj := mining.CreateEmptyBuffer(context, cl.MemReadOnly, 80)
	defer blockHeaderObj.Release()
	kernel.SetArgBuffer(0, blockHeaderObj)

	nonceOutObj := mining.CreateEmptyBuffer(context, cl.MemReadWrite, 8)
	defer nonceOutObj.Release()
	kernel.SetArgBuffer(1, nonceOutObj)

	localItemSize, err := kernel.WorkGroupSize(miner.ClDevice)
	if err != nil {
		log.Fatalln(miner.MinerID, "- WorkGroupSize failed -", err)
	}

	log.Println(miner.MinerID, "- Global item size:", miner.GlobalItemSize, "(Intensity", miner.Intensity, ")", "- Local item size:", localItemSize)

	log.Println(miner.MinerID, "- Initialized ", miner.ClDevice.Type(), "-", miner.ClDevice.Name())

	nonceOut := make([]byte, 8, 8)
	if _, err = commandQueue.EnqueueWriteBufferByte(nonceOutObj, true, 0, nonceOut, nil); err != nil {
		log.Fatalln(miner.MinerID, "-", err)
	}
	for {
		start := time.Now()
		var work *miningWork
		continueMining := true
		select {
		case work, continueMining = <-miner.miningWorkChannel:
		default:
			log.Println(miner.MinerID, "-", "No work ready")
			work, continueMining = <-miner.miningWorkChannel
			log.Println(miner.MinerID, "-", "Continuing")
		}
		if !continueMining {
			log.Println("Halting miner ", miner.MinerID)
			break
		}
		//Copy input to kernel args
		if _, err = commandQueue.EnqueueWriteBufferByte(blockHeaderObj, true, 0, work.Header, nil); err != nil {
			log.Fatalln(miner.MinerID, "-", err)
		}

		//Run the kernel
		if _, err = commandQueue.EnqueueNDRangeKernel(kernel, []int{work.Offset}, []int{miner.GlobalItemSize}, []int{localItemSize}, nil); err != nil {
			log.Fatalln(miner.MinerID, "-", err)
		}
		//Get output
		if _, err = commandQueue.EnqueueReadBufferByte(nonceOutObj, true, 0, nonceOut, nil); err != nil {
			log.Fatalln(miner.MinerID, "-", err)
		}
		//Check if match found
		if nonceOut[0] != 0 || nonceOut[1] != 0 || nonceOut[2] != 0 || nonceOut[3] != 0 || nonceOut[4] != 0 || nonceOut[5] != 0 || nonceOut[6] != 0 || nonceOut[7] != 0 {
			log.Println(miner.MinerID, "-", "Yay, solution found!")

			// Copy nonce to a new header.
			header := append([]byte(nil), work.Header...)
			for i := 0; i < 8; i++ {
				header[i+32] = nonceOut[i]
			}
			go func() {
				if e := miner.Client.SubmitHeader(header, work.Job); e != nil {
					log.Println(miner.MinerID, "- Error submitting solution -", e)
				}
			}()

			//Clear the output since it is dirty now
			nonceOut = make([]byte, 8, 8)
			if _, err = commandQueue.EnqueueWriteBufferByte(nonceOutObj, true, 0, nonceOut, nil); err != nil {
				log.Fatalln(miner.MinerID, "-", err)
			}
		}

		hashRate := float64(miner.GlobalItemSize) / (time.Since(start).Seconds() * 1000000)
		miner.HashRateReports <- &mining.HashRateReport{MinerID: miner.MinerID, HashRate: hashRate}
	}

}
