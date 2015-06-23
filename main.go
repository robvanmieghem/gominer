package main

import (
	"log"

	"github.com/samuel/go-opencl/cl"
)

func main() {

	platforms, err := cl.GetPlatforms()
	if err != nil {
		log.Panic(err)
	}
	for _, platform := range platforms {
		log.Println("Platform", platform.Name())
		devices, err := cl.GetDevices(platform, cl.DeviceTypeAll)
		if err != nil {
			log.Panic(err)
		}
		log.Println(len(devices), "device(s) found:")
		for i, device := range devices {
			log.Println(i, "-", device.Name())
		}
	}
}
