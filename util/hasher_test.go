package util

import (
	"strings"
	"testing"
)

func TestGetFileHash(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name     string
		args     args
		wantHash string
		wantErr  bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHash, err := GetFileHash(tt.args.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetFileHash() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotHash != tt.wantHash {
				t.Errorf("GetFileHash() = %v, want %v", gotHash, tt.wantHash)
			}
		})
	}
}

func TestGetHash(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		input   string
		wantErr error
	}{
		{
			name:    "empty input",
			input:   "",
			want:    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantErr: nil,
		},
		{
			name:    "hello world",
			input:   "hello world",
			want:    "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			got, err := GetHash(reader)
			if err != tt.wantErr {
				t.Errorf("GetHash() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetHash() = %v, want %v", got, tt.want)
			}
		})
	}
}
