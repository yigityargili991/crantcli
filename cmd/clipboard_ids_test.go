package cmd

import (
	"reflect"
	"testing"
)

func TestParseIDs(t *testing.T) {
	got := parseIDs("111, 222\n111\t333")
	want := []string{"111", "222", "333"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIDs() = %#v, want %#v", got, want)
	}
}
