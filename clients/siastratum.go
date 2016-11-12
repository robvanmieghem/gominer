package clients

import (
	"errors"
	"math/big"
)

//SiaStratumClient is a sia client using the stratum protocol
type SiaStratumClient struct {
	connectionstring string
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

//GetHeaderForWork fetches new work from the SIA daemon
func (sc *SiaStratumClient) GetHeaderForWork() (target, header []byte, err error) {
	return
}

//SubmitHeader reports a solved header to the SIA daemon
func (sc *SiaStratumClient) SubmitHeader(header []byte) (err error) {
	return
}
