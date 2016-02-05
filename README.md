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
