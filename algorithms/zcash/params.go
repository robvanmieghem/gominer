package zcash

const (
	equihashParamN                 = 200
	equihashParamK                 = 9
	prefix                         = (equihashParamN / (equihashParamK + 1))
	numberOfInputs                 = (1 << prefix)
	approximateNumberOfElementsLog = (prefix + 1) // Approximate log base 2 of number of elements in hash tables
	numberOfRowsLog                = 20
	overhead                       = 6
	numberOfRows                   = (1 << numberOfRowsLog)
	numberOfSlots                  = ((1 << (approximateNumberOfElementsLog - numberOfRowsLog)) * overhead)
	slotLength                     = 32 //Length of 1 element (slot) in bytes
	htSize                         = (numberOfRows * numberOfSlots * slotLength)
	numberOfZeroBytes              = 12
	zcashHashLength                = 50 //Number of bytes zcash needs out of blake
	//Number of wavefronts per SIMD for the Blake kernel.
	// Blake is ALU-bound (beside the atomic counter being incremented) so we need
	// at least 2 wavefronts per SIMD to hide the 2-clock latency of integer
	// instructions. 10 is the max supported by the hw.
	blakeWPS     = 10
	maxSolutions = 10
)
