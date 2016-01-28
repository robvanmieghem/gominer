# gominer
GPU miner for siacoin in go

All available opencl capable GPU's are detected and used in parallel.

## Prerequisites
* go version 1.4.2 or above (earlier version might work or not), check with `go version`
* opencl libraries on the library path

## Installation
```
go get github.com/robvanmieghem/gominer
```

## Run
```
gominer
```

It is advised to increase the intensity to 28 or something for decent GPU's:
```
gominer -I=28
```
See what intensity gives you the best hashrate.

Commandline arguments with default values:
```
-I=22: Intensity
-cpu=false: If set, also use the CPU for mining, only GPU's are used by default
-h: Show help
```
