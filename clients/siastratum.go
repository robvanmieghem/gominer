package clients

import (
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
	//Subscribe to difficulty changes
	sc.stratumclient.SetNotificationHandler("mining.set_difficulty", func(params []interface{}) {
		if params == nil || len(params) < 1 {
			log.Println("No difficulty parameter supplied by stratum server")
			return
		}
		diff, ok := params[0].(float64)
		if !ok {
			log.Println("Invalid difficulty supplied by stratum server:", params[0])
			return
		}
		log.Println("Stratum server changed difficulty to", diff)
		sc.setDifficulty(diff)
	})
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
		log.Println("Error setting difficulty to ", difficulty)
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
