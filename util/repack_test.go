package util

import "testing"

func TestRepack(t *testing.T) {
	err := CopyToWorkDir("test")
	if err != nil {
		t.Error(err)
	}
}
