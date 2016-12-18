package mining

import (
	"log"

	"github.com/robvanmieghem/go-opencl/cl"
)

//HashRateReport is sent from the mining routines for giving combined information as output
type HashRateReport struct {
	MinerID  int
	HashRate float64
}

//CreateEmptyBuffer calls CreateEmptyBuffer on the supplied context and logs and panics if an error occurred
func CreateEmptyBuffer(ctx *cl.Context, flags cl.MemFlag, size int) (buffer *cl.MemObject) {
	buffer, err := ctx.CreateEmptyBuffer(flags, size)
	if err != nil {
		log.Panicln(err)
	}
	return
}

//Miner declares the common 'Mine' method
type Miner interface {
	Mine()
}
