package clients

import (
	"encoding/hex"
	"errors"
	"log"
	"math/big"
	"reflect"
	"sync"

	"github.com/dchest/blake2b"
	"github.com/robvanmieghem/gominer/clients/stratum"
)

const (
	//HashSize is the length of a sia hash
	HashSize = 32
)

type (
	//Target declares what a solution should be smaller than to be accepted
	Target      [HashSize]byte
	extraNonce2 struct {
		value uint64
		size  uint
	}
)

type stratumJob struct {
	JobID        string
	PrevHash     []byte
	Coinbase1    []byte
	Coinbase2    []byte
	MerkleBranch [][]byte
	Version      string
	NBits        string
	NTime        []byte
	CleanJobs    bool
	ExtraNonce2  extraNonce2
}

//SiaStratumClient is a sia client using the stratum protocol
type SiaStratumClient struct {
	connectionstring string
	User             string

	mutex           sync.Mutex // protects following
	stratumclient   *stratum.Client
	extranonce1     []byte
	extranonce2Size uint
	target          Target
	currentJob      stratumJob
}

//Bytes is a bigendian representation of the extranonce
func (en *extraNonce2) Bytes() (b []byte) {
	b = make([]byte, en.size, en.size)
	for i := uint(0); i < en.size; i++ {
		b[(en.size-1)-i] = byte(en.value >> (i * 8))
	}
	return
}

//Increment increases the nonce with 1, an error is returned if the resulting is value is bigger than possible given the size
func (en *extraNonce2) Increment() (err error) {
	en.value++
	//TODO: check if does not overflow compared to the allowed size
	return
}

func hexStringToBytes(v interface{}) (result []byte, err error) {
	var ok bool
	var stringValue string
	if stringValue, ok = v.(string); !ok {
		return nil, errors.New("Not a valid string")
	}
	if result, err = hex.DecodeString(stringValue); err != nil {
		return nil, errors.New("Not a valid hexadecimal value")
	}
	return
}

//Start connects to the stratumserver and processes the notifications
func (sc *SiaStratumClient) Start() {
	sc.mutex.Lock()
	defer func() {
		sc.mutex.Unlock()
	}()
	sc.stratumclient = &stratum.Client{}
	//In case of an error, drop the current stratumclient and restart
	sc.stratumclient.ErrorCallback = func(err error) {
		log.Println("Error in connection to stratumserver:", err)
		sc.stratumclient.Close()
		sc.Start()
	}

	sc.subscribeToStratumDifficultyChanges()
	sc.subscribeToStratumJobNotifications()

	//Connect to the stratum server
	log.Println("Connecting to", sc.connectionstring)
	sc.stratumclient.Dial(sc.connectionstring)

	//Subscribe for mining
	//Close the connection on an error will cause the client to generate an error, resulting in te errorhandler to be triggered
	reply, err := sc.stratumclient.Call("mining.subscribe", []string{})
	if err != nil || len(reply) < 3 {
		log.Println("ERROR Invalid response from stratum", reply)
		sc.stratumclient.Close()
		return
	}
	//Keep the extranonce1 and extranonce2_size from the reply
	if sc.extranonce1, err = hexStringToBytes(reply[1]); err != nil {
		log.Println("ERROR Invalid extrannonce1 from startum")
		sc.stratumclient.Close()
		return
	}

	extranonce2Size, ok := reply[2].(float64)
	if !ok {
		log.Println("ERROR Invalid extranonce2_size from stratum", reply[2], "type", reflect.TypeOf(reply[2]))
		sc.stratumclient.Close()
		return
	}
	sc.extranonce2Size = uint(extranonce2Size)

}

func (sc *SiaStratumClient) subscribeToStratumDifficultyChanges() {
	sc.stratumclient.SetNotificationHandler("mining.set_difficulty", func(params []interface{}) {
		if params == nil || len(params) < 1 {
			log.Println("ERROR No difficulty parameter supplied by stratum server")
			return
		}
		diff, ok := params[0].(float64)
		if !ok {
			log.Println("ERROR Invalid difficulty supplied by stratum server:", params[0])
			return
		}
		log.Println("Stratum server changed difficulty to", diff)
		sc.setDifficulty(diff)
	})
}

func (sc *SiaStratumClient) subscribeToStratumJobNotifications() {
	sc.stratumclient.SetNotificationHandler("mining.notify", func(params []interface{}) {
		log.Println("New job received from stratum server")
		if params == nil || len(params) < 9 {
			log.Println("ERROR Wrong number of parameters supplied by stratum server")
			return
		}

		sj := stratumJob{}
		sj.ExtraNonce2.size = sc.extranonce2Size

		var ok bool
		var err error
		if sj.JobID, ok = params[0].(string); !ok {
			log.Println("ERROR Wrong job_id parameter supplied by stratum server")
			return
		}
		if sj.PrevHash, err = hexStringToBytes(params[1]); err != nil {
			log.Println("ERROR Wrong prevhash parameter supplied by stratum server")
			return
		}
		if sj.Coinbase1, err = hexStringToBytes(params[2]); err != nil {
			log.Println("ERROR Wrong coinb1 parameter supplied by stratum server")
			return
		}
		if sj.Coinbase2, err = hexStringToBytes(params[3]); err != nil {
			log.Println("ERROR Wrong coinb2 parameter supplied by stratum server")
			return
		}

		//Convert the merklebranch parameter
		merklebranch, ok := params[4].([]interface{})
		if !ok {
			log.Println("ERROR Wrong merkle_branch parameter supplied by stratum server")
			return
		}
		sj.MerkleBranch = make([][]byte, len(merklebranch), len(merklebranch))
		for i, branch := range merklebranch {
			if sj.MerkleBranch[i], err = hexStringToBytes(branch); err != nil {
				log.Println("ERROR Wrong merkle_branch parameter supplied by stratum server")
				return
			}
		}

		if sj.Version, ok = params[5].(string); !ok {
			log.Println("ERROR Wrong version parameter supplied by stratum server")
			return
		}
		if sj.NBits, ok = params[6].(string); !ok {
			log.Println("ERROR Wrong nbits parameter supplied by stratum server")
			return
		}
		if sj.NTime, err = hexStringToBytes(params[7]); err != nil {
			log.Println("ERROR Wrong ntime parameter supplied by stratum server")
			return
		}
		if sj.CleanJobs, ok = params[8].(bool); !ok {
			log.Println("ERROR Wrong clean_jobs parameter supplied by stratum server")
			return
		}
		sc.addNewStratumJob(sj)
	})
}

func (sc *SiaStratumClient) addNewStratumJob(sj stratumJob) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	sc.currentJob = sj
}

// IntToTarget converts a big.Int to a Target.
func intToTarget(i *big.Int) (t Target, err error) {
	// Check for negatives.
	if i.Sign() < 0 {
		err = errors.New("Negative target")
		return
	}
	// In the event of overflow, return the maximum.
	if i.BitLen() > 256 {
		err = errors.New("Target is too high")
		return
	}
	b := i.Bytes()
	offset := len(t[:]) - len(b)
	copy(t[offset:], b)
	return
}

func difficultyToTarget(difficulty float64) (target Target, err error) {
	diffAsBig := big.NewFloat(difficulty)

	diffOneString := "0x00000000ffff0000000000000000000000000000000000000000000000000000"
	targetOneAsBigInt := &big.Int{}
	targetOneAsBigInt.SetString(diffOneString, 0)

	targetAsBigFloat := &big.Float{}
	targetAsBigFloat.SetInt(targetOneAsBigInt)
	targetAsBigFloat.Quo(targetAsBigFloat, diffAsBig)
	targetAsBigInt, _ := targetAsBigFloat.Int(nil)
	target, err = intToTarget(targetAsBigInt)
	return
}

func (sc *SiaStratumClient) setDifficulty(difficulty float64) {
	target, err := difficultyToTarget(difficulty)
	if err != nil {
		log.Println("ERROR Error setting difficulty to ", difficulty)
	}
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	sc.target = target
}

//GetHeaderForWork fetches new work from the SIA daemon
func (sc *SiaStratumClient) GetHeaderForWork() (target, header []byte, job interface{}, err error) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	job = sc.currentJob

	if sc.currentJob.JobID == "" {
		err = errors.New("No job received from stratum server yet")
	}

	target = sc.target[:]

	//Create the arbitrary transaction
	en2 := sc.currentJob.ExtraNonce2.Bytes()
	err = sc.currentJob.ExtraNonce2.Increment()

	arbtx := []byte{0}
	arbtx = append(arbtx, sc.currentJob.Coinbase1...)
	arbtx = append(arbtx, sc.extranonce1...)
	arbtx = append(arbtx, en2...)
	arbtx = append(arbtx, sc.currentJob.Coinbase2...)
	arbtxHash := blake2b.Sum256(arbtx)

	//Construct the merkleroot from the arbitrary transaction and the merklebranches
	merkleRoot := arbtxHash
	for _, h := range sc.currentJob.MerkleBranch {
		m := append([]byte{1}[:], h...)
		m = append(m, merkleRoot[:]...)
		merkleRoot = blake2b.Sum256(m)
	}

	//Construct the header
	header = make([]byte, 0, 80)
	header = append(header, sc.currentJob.PrevHash...)
	header = append(header, []byte{0, 0, 0, 0, 0, 0, 0, 0}[:]...) //empty nonce
	header = append(header, sc.currentJob.NTime...)
	header = append(header, merkleRoot[:]...)

	return
}

//SubmitHeader reports a solved header to the SIA daemon
func (sc *SiaStratumClient) SubmitHeader(header []byte, job interface{}) (err error) {
	sj, _ := job.(stratumJob)
	nonce := hex.EncodeToString(header[32:40])
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	encodedExtraNonce2 := hex.EncodeToString(sj.ExtraNonce2.Bytes())
	nTime := hex.EncodeToString(sj.NTime)
	result, err := sc.stratumclient.Call("mining.submit", []string{sc.User, sj.JobID, encodedExtraNonce2, nTime, nonce})
	if err != nil {
		return
	}
	log.Println(result)
	return
}
