package clients

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"reflect"
	"sync"

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
	PrevHash     string
	Coinbase1    string
	Coinbase2    string
	MerkleBranch []string
	Version      string
	NBits        string
	NTime        string
	CleanJobs    bool
	ExtraNonce2  extraNonce2
}

//SiaStratumClient is a sia client using the stratum protocol
type SiaStratumClient struct {
	connectionstring string

	mutex           sync.Mutex // protects following
	stratumclient   *stratum.Client
	extranonce1     string
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
	var ok bool
	if sc.extranonce1, ok = reply[1].(string); !ok {
		log.Println("ERROR Invalid extranonce1 from stratum", reply)
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
		if params == nil || len(params) < 9 {
			log.Println("ERROR Wrong number of parameters supplied by stratum server")
			return
		}

		sj := stratumJob{}
		sj.ExtraNonce2.size = sc.extranonce2Size

		var ok bool
		if sj.JobID, ok = params[0].(string); !ok {
			log.Println("ERROR Wrong job_id parameter supplied by stratum server")
			return
		}
		if sj.PrevHash, ok = params[1].(string); !ok {
			log.Println("ERROR Wrong prevhash parameter supplied by stratum server")
			return
		}
		if sj.Coinbase1, ok = params[2].(string); !ok {
			log.Println("ERROR Wrong coinb1 parameter supplied by stratum server")
			return
		}
		if sj.Coinbase2, ok = params[3].(string); !ok {
			log.Println("ERROR Wrong coinb2 parameter supplied by stratum server")
			return
		}

		//Convert the merklebranch parameter
		merklebranch, ok := params[4].([]interface{})
		if !ok {
			log.Println("ERROR Wrong merkle_branch parameter supplied by stratum server")
			return
		}
		sj.MerkleBranch = make([]string, len(merklebranch), len(merklebranch))
		for i, branch := range merklebranch {
			sj.MerkleBranch[i], ok = branch.(string)
			if !ok {
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
		if sj.NTime, ok = params[7].(string); !ok {
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
func (sc *SiaStratumClient) GetHeaderForWork() (target, header []byte, err error) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	target = sc.target[:]
	en2 := sc.currentJob.ExtraNonce2.Bytes()
	sc.currentJob.ExtraNonce2.Increment()

	fmt.Println("Constructing arbitrary tx, extranonce2:", hex.EncodeToString(en2))

	err = errors.New("Not implemented yet")
	return
}

//SubmitHeader reports a solved header to the SIA daemon
func (sc *SiaStratumClient) SubmitHeader(header []byte) (err error) {
	return
}
