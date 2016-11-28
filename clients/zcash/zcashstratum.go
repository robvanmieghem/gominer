package zcash

import (
	"errors"
	"log"
	"strings"
	"sync"

	"github.com/robvanmieghem/gominer/clients"
	"github.com/robvanmieghem/gominer/clients/stratum"
)

// zcash stratum as defined on https://github.com/str4d/zips/blob/23d74b0373c824dd51c7854c0e3ea22489ba1b76/drafts/str4d-stratum/draft1.rst

//StratumClient is a zcash client using the stratum protocol
type StratumClient struct {
	connectionstring string
	User             string

	mutex           sync.Mutex // protects following
	stratumclient   *stratum.Client
	extranonce1     []byte
	extranonce2Size uint
	target          []byte
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
		log.Println("ERROR Invalid extrannonce1 from startum")
		sc.stratumclient.Close()
		return
	}

	sc.extranonce2Size = uint(32 - len(sc.extranonce1))

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
		log.Println("Job notifications not implemented yet")
	})
}

//GetHeaderForWork fetches new work from the SIA daemon
func (sc *StratumClient) GetHeaderForWork() (target, header []byte, deprecationChannel chan bool, job interface{}, err error) {
	err = errors.New("GetHeaderForWork not implemented for zcash stratum yet")
	return
}

//SubmitHeader reports a solved header
func (sc *StratumClient) SubmitHeader(header []byte, job interface{}) (err error) {
	err = errors.New("SubmitHeader not implemented for zcash stratum yet")
	return
}
