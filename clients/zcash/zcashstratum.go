package zcash

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log"
	"strings"
	"sync"

	"github.com/robvanmieghem/gominer/clients"
	"github.com/robvanmieghem/gominer/clients/stratum"
)

// zcash stratum as defined on https://github.com/str4d/zips/blob/23d74b0373c824dd51c7854c0e3ea22489ba1b76/drafts/str4d-stratum/draft1.rst

type stratumJob struct {
	JobID      string
	Version    []byte
	PrevHash   []byte
	MerkleRoot []byte
	Reserved   []byte
	Time       []byte
	Bits       []byte
	CleanJobs  bool

	ExtraNonce2 stratum.ExtraNonce2
}

//StratumClient is a zcash client using the stratum protocol
type StratumClient struct {
	connectionstring string
	User             string

	mutex           sync.Mutex // protects following
	stratumclient   *stratum.Client
	extranonce1     []byte
	extranonce2Size uint
	target          []byte
	currentJob      stratumJob
	clients.BaseClient
}

// NewClient creates a new StratumClient given a '[stratum+tcp://]host:port' connectionstring
func NewClient(connectionstring, pooluser string) (sc clients.Client) {
	if strings.HasPrefix(connectionstring, "stratum+tcp://") {
		connectionstring = strings.TrimPrefix(connectionstring, "stratum+tcp://")
	}
	sc = &StratumClient{connectionstring: connectionstring, User: pooluser}
	return
}

//Start connects to the stratumserver and processes the notifications
func (sc *StratumClient) Start() {
	sc.mutex.Lock()
	defer func() {
		sc.mutex.Unlock()
	}()

	sc.DeprecateOutstandingJobs()

	sc.stratumclient = &stratum.Client{}
	//In case of an error, drop the current stratumclient and restart
	sc.stratumclient.ErrorCallback = func(err error) {
		log.Println("Error in connection to stratumserver:", err)
		sc.stratumclient.Close()
		sc.Start()
	}

	sc.subscribeToStratumTargetChanges()
	sc.subscribeToStratumJobNotifications()

	//Connect to the stratum server
	log.Println("Connecting to", sc.connectionstring)
	sc.stratumclient.Dial(sc.connectionstring)

	//Subscribe for mining
	//Close the connection on an error will cause the client to generate an error, resulting in te errorhandler to be triggered
	result, err := sc.stratumclient.Call("mining.subscribe", []string{"gominer"})
	if err != nil {
		log.Println("ERROR Error in response from stratum", err)
		sc.stratumclient.Close()
		return
	}
	reply, ok := result.([]interface{})
	if !ok || len(reply) < 2 {
		log.Println("ERROR Invalid response from stratum", result)
		sc.stratumclient.Close()
		return
	}

	//Keep the extranonce1 and extranonce2_size from the reply
	if sc.extranonce1, err = stratum.HexStringToBytes(reply[1]); err != nil {
		log.Println("ERROR Invalid extranonce1 from startum")
		sc.stratumclient.Close()
		return
	}

	sc.extranonce2Size = uint(32 - len(sc.extranonce1))
	if sc.extranonce2Size < 15 {
		log.Println("ERROR Incompatible server, nonce1 too long")
		sc.stratumclient.Close()
		return
	}

	//Authorize the miner
	_, err = sc.stratumclient.Call("mining.authorize", []string{sc.User, ""})
	if err != nil {
		log.Println("Unable to authorize:", err)
		sc.stratumclient.Close()
		return
	}

}

func (sc *StratumClient) subscribeToStratumTargetChanges() {
	sc.stratumclient.SetNotificationHandler("mining.set_target", func(params []interface{}) {

		if params == nil || len(params) < 1 {
			log.Println("ERROR No target parameter supplied by stratum server")
			return
		}
		var err error
		sc.target, err = stratum.HexStringToBytes(params[0])
		if err != nil {
			log.Println("ERROR Invalid target supplied by stratum server:", params[0])
		}

		log.Println("Stratum server changed target to", params[0])
	})
}

func (sc *StratumClient) subscribeToStratumJobNotifications() {
	sc.stratumclient.SetNotificationHandler("mining.notify", func(params []interface{}) {
		log.Println("New job received from stratum server")
		if params == nil || len(params) < 8 {
			log.Println("ERROR Wrong number of parameters supplied by stratum server")
			return
		}

		sj := stratumJob{}

		sj.ExtraNonce2.Size = sc.extranonce2Size

		var ok bool
		var err error
		if sj.JobID, ok = params[0].(string); !ok {
			log.Println("ERROR Wrong job_id parameter supplied by stratum server")
			return
		}
		if sj.Version, err = stratum.HexStringToBytes(params[1]); err != nil {
			log.Println("ERROR Wrong version parameter supplied by stratum server:", params[1])
			return
		}
		v := binary.LittleEndian.Uint32(sj.Version)
		if v != 4 {
			log.Println("ERROR Wrong version supplied by stratum server:", sj.Version)
			return
		}
		if sj.PrevHash, err = stratum.HexStringToBytes(params[2]); err != nil {
			log.Println("ERROR Wrong prevhash parameter supplied by stratum server")
			return
		}
		if sj.MerkleRoot, err = stratum.HexStringToBytes(params[3]); err != nil {
			log.Println("ERROR Wrong merkleroot parameter supplied by stratum server")
			return
		}
		if sj.Reserved, err = stratum.HexStringToBytes(params[5]); err != nil {
			log.Println("ERROR Wrong reserved parameter supplied by stratum server")
			return
		}
		if sj.Time, err = stratum.HexStringToBytes(params[5]); err != nil {
			log.Println("ERROR Wrong time parameter supplied by stratum server")
			return
		}

		if sj.Bits, err = stratum.HexStringToBytes(params[6]); err != nil {
			log.Println("ERROR Wrong bits parameter supplied by stratum server")
			return
		}
		if sj.CleanJobs, ok = params[7].(bool); !ok {
			log.Println("ERROR Wrong clean_jobs parameter supplied by stratum server")
			return
		}
		sc.addNewStratumJob(sj)
	})
}

func (sc *StratumClient) addNewStratumJob(sj stratumJob) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	sc.currentJob = sj
	if sj.CleanJobs {
		sc.DeprecateOutstandingJobs()
	}
	sc.AddJobToDeprecate(sj.JobID)
}

//GetHeaderForWork fetches new work from the SIA daemon
func (sc *StratumClient) GetHeaderForWork() (target, header []byte, deprecationChannel chan bool, job interface{}, err error) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	job = sc.currentJob
	if sc.currentJob.JobID == "" {
		err = errors.New("No job received from stratum server yet")
		return
	}

	deprecationChannel = sc.GetDeprecationChannel(sc.currentJob.JobID)

	target = sc.target

	nonceLessHeader := make([]byte, 0, 108)
	nonceLessHeader = append(nonceLessHeader, sc.currentJob.Version...)    // 4 bytes
	nonceLessHeader = append(nonceLessHeader, sc.currentJob.PrevHash...)   // 32 bytes
	nonceLessHeader = append(nonceLessHeader, sc.currentJob.MerkleRoot...) // 32 bytes
	nonceLessHeader = append(nonceLessHeader, sc.currentJob.Reserved...)   // 32 bytes
	nonceLessHeader = append(nonceLessHeader, sc.currentJob.Time...)       // 4 bytes
	nonceLessHeader = append(nonceLessHeader, sc.currentJob.Bits...)       // 4 bytes

	header = append(nonceLessHeader, sc.extranonce1...)

	err = errors.New("GetHeaderForWork not implemented for zcash stratum yet")
	return
}

//SubmitHeader reports a solved header
func (sc *StratumClient) SubmitHeader(header []byte, job interface{}) (err error) {
	sj, _ := job.(stratumJob)
	//TODO: extract nonce and equihash_solution from the header
	equihashsolution := "00"
	encodedExtraNonce2 := hex.EncodeToString(sj.ExtraNonce2.Bytes())
	nTime := hex.EncodeToString(sj.Time)

	sc.mutex.Lock()
	c := sc.stratumclient
	sc.mutex.Unlock()
	_, err = c.Call("mining.submit", []string{sc.User, sj.JobID, nTime, encodedExtraNonce2, equihashsolution})
	if err != nil {
		return
	}
	return
}
