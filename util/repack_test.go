package util

import "testing"

func TestRepack(t *testing.T) {
	err := CopyToWorkDir("test")
	if err != nil {
		t.Error(err)
	}

	t.Error(GCWorkDirs())
	t.Fail()
}

// TODO: finish writing test?
//looks like this is where it was left off
