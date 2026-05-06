package auth

import (
	"bytes"
	"testing"
)

func TestNewTokenReturnsThirtyTwoRandomBytes(t *testing.T) {
	token, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if len(token) != 32 {
		t.Fatalf("NewToken length = %d, want 32", len(token))
	}
	if bytes.Equal(token, make([]byte, 32)) {
		t.Fatal("NewToken returned an all-zero token")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		[]byte("bootstrap-token"),
		[]byte{0x00, 0x01, 0x02, 0xfd, 0xfe, 0xff},
	}

	for _, tc := range cases {
		encoded := Encode(tc)
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatalf("Decode(Encode(%v)): %v", tc, err)
		}
		if !bytes.Equal(decoded, tc) {
			t.Fatalf("Decode(Encode(%v)) = %v", tc, decoded)
		}
	}
}

func TestDecodeRejectsMalformedBase64(t *testing.T) {
	for _, input := range []string{
		"not base64",
		"!!!!",
		"abc",
		"abcd=",
	} {
		if got, err := Decode(input); err == nil {
			t.Fatalf("Decode(%q) = %v, nil error", input, got)
		}
	}
}

func TestEqualToken(t *testing.T) {
	left := []byte("same length token")
	if !EqualToken(left, []byte("same length token")) {
		t.Fatal("EqualToken rejected identical token bytes")
	}
	if EqualToken(left, []byte("same length tokem")) {
		t.Fatal("EqualToken accepted different token bytes with the same length")
	}
	if EqualToken(left, []byte("short")) {
		t.Fatal("EqualToken accepted different-length token bytes")
	}
	if !EqualToken(nil, nil) {
		t.Fatal("EqualToken rejected two nil tokens")
	}
}
