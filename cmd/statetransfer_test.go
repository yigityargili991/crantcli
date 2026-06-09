package cmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseIDsValidatesAndDeduplicatesUint64(t *testing.T) {
	got, err := parseIDs(" 720575940610453042, 42\n0042 18446744073709551615 ")
	if err != nil {
		t.Fatalf("parseIDs returned error: %v", err)
	}
	want := []string{"720575940610453042", "42", "18446744073709551615"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIDs = %#v, want %#v", got, want)
	}
}

func TestParseIDsRejectsInvalidToken(t *testing.T) {
	_, err := parseIDs("123 abc")
	if err == nil {
		t.Fatal("expected invalid token error")
	}
	if !strings.Contains(err.Error(), `invalid clipboard root ID "abc": expected unsigned decimal uint64`) {
		t.Fatalf("error = %q, want invalid-token guidance", err.Error())
	}
}

func TestParseIDsRejectsOverflow(t *testing.T) {
	_, err := parseIDs("18446744073709551616")
	if err == nil {
		t.Fatal("expected overflow error")
	}
	if !strings.Contains(err.Error(), `invalid clipboard root ID "18446744073709551616": exceeds uint64 range`) {
		t.Fatalf("error = %q, want overflow guidance", err.Error())
	}
}
