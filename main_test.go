package main

import "testing"

func TestExcludedDevices(t *testing.T) {
	testSet := []struct {
		deviceID     int
		excludedGPUs string
		excluded     bool
	}{{
		deviceID:     1,
		excludedGPUs: "",
		excluded:     false,
	},
		{
			deviceID:     2,
			excludedGPUs: "2",
			excluded:     true,
		},
		{
			deviceID:     2,
			excludedGPUs: "3,2",
			excluded:     true,
		},
		{
			deviceID:     1,
			excludedGPUs: "2,3",
			excluded:     false,
		},
		{
			deviceID:     1,
			excludedGPUs: "0",
			excluded:     false,
		},
	}
	for _, test := range testSet {
		result := deviceExcludedForMining(test.deviceID, test.excludedGPUs)
		if result != test.excluded {
			t.Error(test)
		}
	}
}
