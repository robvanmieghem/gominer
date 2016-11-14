package clients

import (
	"encoding/json"
	"errors"
	"log"
	"math/big"
	"sync"

	"github.com/robvanmieghem/gominer/clients/stratum"
)

//SiaStratumClient is a sia client using the stratum protocol
type SiaStratumClient struct {
	connectionstring   string
	stratumclientMutex sync.Mutex // protects following
	stratumclient      *stratum.Client
	target             Target
}

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
}

const (
	//HashSize is the length of a sia hash
	HashSize = 32
)

type (
	//Target declares what a solution should be smaller than to be accepted
	Target [HashSize]byte
)

//Start connects to the stratumserver and processes the notifications
func (sc *SiaStratumClient) Start() {
	sc.stratumclientMutex.Lock()
	defer func() {
		sc.stratumclientMutex.Unlock()
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
	//TODO: proper response handling
	err := sc.stratumclient.Call("mining.subscribe", []string{}, nil)
	if err != nil {
		//Closing the connection will cause the client to generate an error, resulting in te errorhandler to be triggered
		sc.stratumclient.Close()
	}
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
		var ok bool
		if sj.JobID, ok = params[0].(string); !ok {
			log.Println("Wrong job_id parameter supplied by stratum server")
			return
		}
		if sj.PrevHash, ok = params[1].(string); !ok {
			log.Println("Wrong prevhash parameter supplied by stratum server")
			return
		}
		if sj.Coinbase1, ok = params[2].(string); !ok {
			log.Println("Wrong coinb1 parameter supplied by stratum server")
			return
		}
		if sj.Coinbase2, ok = params[3].(string); !ok {
			log.Println("Wrong coinb2 parameter supplied by stratum server")
			return
		}

		//Convert the merklebranch parameter
		merklebranch, ok := params[4].([]interface{})
		if !ok {
			log.Println("Wrong merkle_branch parameter supplied by stratum server")
			return
		}
		sj.MerkleBranch = make([]string, len(merklebranch), len(merklebranch))
		for i, branch := range merklebranch {
			sj.MerkleBranch[i], ok = branch.(string)
			if !ok {
				log.Println("Wrong merkle_branch parameter supplied by stratum server")
				return
			}
		}

		if sj.Version, ok = params[5].(string); !ok {
			log.Println("Wrong version parameter supplied by stratum server")
			return
		}
		if sj.NBits, ok = params[6].(string); !ok {
			log.Println("Wrong nbits parameter supplied by stratum server")
			return
		}
		if sj.NTime, ok = params[7].(string); !ok {
			log.Println("Wrong ntime parameter supplied by stratum server")
			return
		}
		if sj.CleanJobs, ok = params[8].(bool); !ok {
			log.Println("Wrong clean_jobs parameter supplied by stratum server")
			return
		}
		sc.addNewStratumJob(sj)
	})
}

func (sc *SiaStratumClient) addNewStratumJob(sj stratumJob) {
	sjb, _ := json.Marshal(sj)
	log.Println("DEBUG: job received from stratum server:", string(sjb))

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
	sc.target = target
}

//GetHeaderForWork fetches new work from the SIA daemon
func (sc *SiaStratumClient) GetHeaderForWork() (target, header []byte, err error) {
	err = errors.New("Not implemented yet")
	return
}

//SubmitHeader reports a solved header to the SIA daemon
func (sc *SiaStratumClient) SubmitHeader(header []byte) (err error) {
	return
}
