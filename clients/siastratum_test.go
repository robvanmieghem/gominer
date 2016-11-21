package clients

import (
	"encoding/hex"
	"strconv"
	"testing"
)

func TestDifficultyToTarget(t *testing.T) {
	diff, _ := strconv.ParseFloat("0.99998474121094105", 64)

	expectedTarget := "0x00000000fffffffffffefffeffff00000001000200020000fffefffcfffbfffd"

	target, err := difficultyToTarget(diff)
	if err != nil {
		t.Error(err)
	}

	if expectedTarget != ("0x" + hex.EncodeToString(target[:])) {
		t.Error("0x"+hex.EncodeToString(target[:]), "returned instead of", expectedTarget)
	}
}

func TestExtraNonce2(t *testing.T) {
	// Test serialization
	en := extraNonce2{value: 1, size: 4}
	expected := "00000001"
	result := hex.EncodeToString(en.Bytes())
	if expected != result {
		t.Error(result, "returned instead of", expected)
	}

	//Test increment
	en = extraNonce2{value: 1, size: 4}
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
