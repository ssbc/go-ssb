package ssb

import "testing"

func TestSignatureVerify(t *testing.T) {
	n := min(20, len(testMessages))
	for i := 1; i < n; i++ {
		enc, sig, err := EncodePreserveOrder(testMessages[i].Input)
		if err != nil {
			t.Fatal(err)
		}
		if sig.Algo() != SigAlgoEd25519 {
			t.Errorf("Expected Ed25519 algo. Got: %v", sig.Algo())
		}
		err = sig.Verify(enc, *testMessages[i].Author)
		if err != nil {
			t.Errorf("Signature Verification failed: %s", err)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
