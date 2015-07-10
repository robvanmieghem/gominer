package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/robvanmieghem/go-opencl/cl"
)

var kernelSource = `
static inline ulong rotr64( __const ulong w, __const unsigned c ) { return ( w >> c ) | ( w << ( 64 - c ) ); }

__constant static const uchar blake2b_sigma[12][16] = {
	{ 0,  1,  2,  3,  4,  5,  6,  7,  8,  9,  10, 11, 12, 13, 14, 15 } ,
	{ 14, 10, 4,  8,  9,  15, 13, 6,  1,  12, 0,  2,  11, 7,  5,  3  } ,
	{ 11, 8,  12, 0,  5,  2,  15, 13, 10, 14, 3,  6,  7,  1,  9,  4  } ,
	{ 7,  9,  3,  1,  13, 12, 11, 14, 2,  6,  5,  10, 4,  0,  15, 8  } ,
	{ 9,  0,  5,  7,  2,  4,  10, 15, 14, 1,  11, 12, 6,  8,  3,  13 } ,
	{ 2,  12, 6,  10, 0,  11, 8,  3,  4,  13, 7,  5,  15, 14, 1,  9  } ,
	{ 12, 5,  1,  15, 14, 13, 4,  10, 0,  7,  6,  3,  9,  2,  8,  11 } ,
	{ 13, 11, 7,  14, 12, 1,  3,  9,  5,  0,  15, 4,  8,  6,  2,  10 } ,
	{ 6,  15, 14, 9,  11, 3,  0,  8,  12, 2,  13, 7,  1,  4,  10, 5  } ,
	{ 10, 2,  8,  4,  7,  6,  1,  5,  15, 11, 9,  14, 3,  12, 13, 0  } ,
	{ 0,  1,  2,  3,  4,  5,  6,  7,  8,  9,  10, 11, 12, 13, 14, 15 } ,
	{ 14, 10, 4,  8,  9,  15, 13, 6,  1,  12, 0,  2,  11, 7,  5,  3  } };

// Target is passed in via headerIn[32 - 29]
__kernel void nonceGrind(__global ulong *headerIn, __global ulong *nonceOut) {
	ulong target = headerIn[4];
	ulong m[16] = {	headerIn[0], headerIn[1],
	                headerIn[2], headerIn[3],
	                (ulong)get_global_id(0), headerIn[5],
	                headerIn[6], headerIn[7],
	                headerIn[8], headerIn[9], 0, 0, 0, 0, 0, 0 };

	ulong v[16] = { 0x6a09e667f2bdc928, 0xbb67ae8584caa73b, 0x3c6ef372fe94f82b, 0xa54ff53a5f1d36f1,
	                0x510e527fade682d1, 0x9b05688c2b3e6c1f, 0x1f83d9abfb41bd6b, 0x5be0cd19137e2179,
	                0x6a09e667f3bcc908, 0xbb67ae8584caa73b, 0x3c6ef372fe94f82b, 0xa54ff53a5f1d36f1,
	                0x510e527fade68281, 0x9b05688c2b3e6c1f, 0xe07c265404be4294, 0x5be0cd19137e2179 };



#define G(r,i,a,b,c,d) \
	a = a + b + m[blake2b_sigma[r][2*i]]; \
	d = rotr64(d ^ a, 32); \
	c = c + d; \
	b = rotr64(b ^ c, 24); \
	a = a + b + m[blake2b_sigma[r][2*i+1]]; \
	d = rotr64(d ^ a, 16); \
	c = c + d; \
	b = rotr64(b ^ c, 63);

#define ROUND(r)                    \
	G(r,0,v[ 0],v[ 4],v[ 8],v[12]); \
	G(r,1,v[ 1],v[ 5],v[ 9],v[13]); \
	G(r,2,v[ 2],v[ 6],v[10],v[14]); \
	G(r,3,v[ 3],v[ 7],v[11],v[15]); \
	G(r,4,v[ 0],v[ 5],v[10],v[15]); \
	G(r,5,v[ 1],v[ 6],v[11],v[12]); \
	G(r,6,v[ 2],v[ 7],v[ 8],v[13]); \
	G(r,7,v[ 3],v[ 4],v[ 9],v[14]);

	ROUND( 0 );
	ROUND( 1 );
	ROUND( 2 );
	ROUND( 3 );
	ROUND( 4 );
	ROUND( 5 );
	ROUND( 6 );
	ROUND( 7 );
	ROUND( 8 );
	ROUND( 9 );
	ROUND( 10 );
	ROUND( 11 );
#undef G
#undef ROUND

	if (as_ulong(as_uchar8(0x6a09e667f2bdc928 ^ v[0] ^ v[8]).s76543210) < target) {
		*nonceOut = m[4];
		return;
	}
}
`

var getworkurl = "http://localhost:9980/miner/headerforwork"
var submitblockurl = "http://localhost:9980/miner/submitheader"
var intensity = 22
var devicesTypesForMining = cl.DeviceTypeGPU

func loadCLProgramSource() (sources []string) {
	filename := "sia-gpu-miner.cl"

	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	sources = []string{string(buf)}
	return
}

func getHeaderForWork() (target, header []byte, err error) {
	resp, err := http.Get(getworkurl)
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
	//TODO: check for invalid target
	return
}

func submitHeader(header []byte) (err error) {
	_, err = http.Post(submitblockurl, "", bytes.NewReader(header))
	return
}

func mine(clDevice *cl.Device, minerID int) {
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

	globalItemSize := int(math.Exp2(float64(intensity)))
	log.Println(minerID, "- global item size:", globalItemSize, "- local item size:", localItemSize)

	log.Println(minerID, "- Started mining on", clDevice.Type(), "-", clDevice.Name())

	for {
		start := time.Now()

		target, header, err := getHeaderForWork()
		if err != nil {
			log.Println(minerID, "- ERROR ", err)
			continue
		}
		//copy target to header
		for i := 0; i < 8; i++ {
			header[i+32] = target[7-i]
		}
		//TODO: offset

		//Copy input to kernel args
		if _, err = commandQueue.EnqueueWriteBufferByte(blockHeaderObj, true, 0, header, nil); err != nil {
			log.Fatalln(minerID, "-", err)
		}

		nonceOut := make([]byte, 8, 8) //TODO: get this out of the for loop
		if _, err = commandQueue.EnqueueWriteBufferByte(nonceOutObj, true, 0, nonceOut, nil); err != nil {
			log.Fatalln(minerID, "-", err)
		}

		//Run the kernel
		//globalIDOffset := globalItemSize
		if _, err = commandQueue.EnqueueNDRangeKernel(kernel, nil, []int{globalItemSize}, []int{localItemSize}, nil); err != nil {
			log.Fatalln(minerID, "-", err)
		}
		//Get output
		if _, err = commandQueue.EnqueueReadBufferByte(nonceOutObj, true, 0, nonceOut, nil); err != nil {
			log.Fatalln(minerID, "_", err)
		}
		//Check if match found
		if nonceOut[0] != 0 {
			log.Println(minerID, "-", "Yay, block found!")
			// Copy nonce to header.
			for i := 0; i < 8; i++ {
				header[i+32] = nonceOut[i]
			}
			if err = submitHeader(header); err != nil {
				log.Println(minerID, "- Error submitting block -", err)
			}
		}

		hashRate := float64(globalItemSize) / (time.Since(start).Seconds() * 1000000)
		fmt.Printf("\r%d - Mining at %.3f MH/s", minerID, hashRate)

	}

}

func main() {
	useCPU := flag.Bool("cpu", false, "If set, also use the CPU for mining, only GPU's are used by default")
	flag.IntVar(&intensity, "I", intensity, "Intensity")

	flag.Parse()

	if *useCPU {
		devicesTypesForMining = cl.DeviceTypeAll
	}

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
	if len(clDevices) == 0 {
		log.Println("No suitable opencl devices found")
		os.Exit(1)
	}

	for i, device := range clDevices {
		go mine(device, i)
	}

	var input string
	fmt.Scanln(&input)
}
