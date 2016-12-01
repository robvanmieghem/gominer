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
)
