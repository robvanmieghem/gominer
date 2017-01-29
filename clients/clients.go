//Package clients provides some utilities and common code for specific client implementations
package clients

//HeaderReporter defines the required method a SIA client or pool client should implement for miners to be able to report solved headers
type HeaderReporter interface {
	//SubmitHeader reports a solved header
	SubmitHeader(header []byte, job interface{}) (err error)
}

//HeaderProvider supplies headers for a miner to mine on
type HeaderProvider interface {
	//GetHeaderForWork providers a header to mine on
	// the deprecationChannel is closed when the job should be abandoned
	GetHeaderForWork() (target, header []byte, deprecationChannel chan bool, job interface{}, err error)
}

//DeprecatedJobCall is a function that can be registered on a client to be executed when
// the server indicates that all previous jobs should be abandoned
type DeprecatedJobCall func()

// Client defines the interface for a client towards a work provider
type Client interface {
	HeaderProvider
	HeaderReporter
	//Start connects to a sia daemon and starts supplying valid headers
	// It can be empty in case of a "getwork" implementation or maintain a tcp connection in case of stratum for example
	Start()
	//SetDeprecatedJobCall sets the function to be called when the previous jobs should be abandoned
	SetDeprecatedJobCall(call DeprecatedJobCall)
}

//BaseClient implements some common properties and functionality
type BaseClient struct {
	deprecationChannels map[string]chan bool

	deprecatedJobCall DeprecatedJobCall
}

//DeprecateOutstandingJobs closes all deprecationChannels and removes them from the list
// This method is not threadsafe
func (sc *BaseClient) DeprecateOutstandingJobs() {
	if sc.deprecationChannels == nil {
		sc.deprecationChannels = make(map[string]chan bool)
	}
	for jobid, deprecatedJob := range sc.deprecationChannels {
		close(deprecatedJob)
		delete(sc.deprecationChannels, jobid)
	}
	call := sc.deprecatedJobCall
	if call != nil {
		go call()
	}
}

// AddJobToDeprecate add the jobid to the list of jobs that should be deprecated when the times comes
func (sc *BaseClient) AddJobToDeprecate(jobid string) {
	sc.deprecationChannels[jobid] = make(chan bool)
}

// GetDeprecationChannel return the channel that will be closed when a job gets deprecated
func (sc *BaseClient) GetDeprecationChannel(jobid string) chan bool {
	return sc.deprecationChannels[jobid]
}

//SetDeprecatedJobCall sets the function to be called when the previous jobs should be abandoned
func (sc *BaseClient) SetDeprecatedJobCall(call DeprecatedJobCall) {
	sc.deprecatedJobCall = call
}
