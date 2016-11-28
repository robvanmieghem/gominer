package stratum

import (
	"encoding/hex"
	"testing"
)

func TestExtraNonce2(t *testing.T) {
	// Test serialization
	en := ExtraNonce2{Value: 1, Size: 4}
	expected := "00000001"
	result := hex.EncodeToString(en.Bytes())
	if expected != result {
		t.Error(result, "returned instead of", expected)
	}

	//Test increment
	en = ExtraNonce2{Value: 1, Size: 4}
	err := en.Increment()
	if err != nil {
		t.Error("Error from the increment call:", err)
		return
	}
	expected = "00000002"
	result = hex.EncodeToString(en.Bytes())
	if expected != result {
		t.Error(result, "returned instead of", expected)
	}
}
