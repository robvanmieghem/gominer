# gominer
GPU miner for siacoin in go

All available opencl capable GPU's are detected and used in parallel.


## Installation from source

### Prerequisites
* go version 1.4.2 or above (earlier version might work or not), check with `go version`
* opencl libraries on the library path
* gcc

```
go get github.com/robvanmieghem/gominer
```

## Run
```
gominer
```

Commandline arguments with default values:
```
-I=28: Intensity
-cpu=false: If set, also use the CPU for mining, only GPU's are used by default
-v: Show version and exit
-h: Show help
```

See what intensity gives you the best hashrate.

## Support development

If you really want to, you can support the go-miner development:

SIA address: 79b9089439218734192db7016f07dc5a0e2a95e873992dd782a1e1306b2c44e116e1d8ded910
