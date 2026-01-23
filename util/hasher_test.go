package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetFileHash(t *testing.T) {
	// Create temp directory for test files
	tmpDir := t.TempDir()

	// Create test files with known content
	emptyFile := filepath.Join(tmpDir, "empty.txt")
	os.WriteFile(emptyFile, []byte{}, 0644)

	helloFile := filepath.Join(tmpDir, "hello.txt")
	os.WriteFile(helloFile, []byte("hello world"), 0644)

	binaryFile := filepath.Join(tmpDir, "binary.bin")
	os.WriteFile(binaryFile, []byte{0x00, 0x01, 0x02, 0xff}, 0644)

	subDir := filepath.Join(tmpDir, "subdir")
	os.Mkdir(subDir, 0755)

	tests := []struct {
		name     string
		path     string
		wantHash string
		wantErr  error
	}{
		{
			name:     "empty file",
			path:     emptyFile,
			wantHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantErr:  nil,
		},
		{
			name:     "hello world file",
			path:     helloFile,
			wantHash: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			wantErr:  nil,
		},
		{
			name:     "binary file",
			path:     binaryFile,
			wantHash: "3d1f57c984978ef98a18378c8166c1cb8ede02c03eeb6aee7e2f121dfeee3e56",
			wantErr:  nil,
		},
		{
			name:    "directory returns error",
			path:    subDir,
			wantErr: ErrExpectedFile,
		},
		{
			name:    "non-existent file",
			path:    filepath.Join(tmpDir, "nonexistent.txt"),
			wantErr: os.ErrNotExist, // Will be wrapped, check with errors.Is
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHash, err := GetFileHash(tt.path)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("GetFileHash() expected error %v, got nil", tt.wantErr)
					return
				}
				// For os.ErrNotExist, the error is wrapped
				if tt.wantErr == os.ErrNotExist {
					if !os.IsNotExist(err) {
						t.Errorf("GetFileHash() error = %v, want os.ErrNotExist", err)
					}
					return
				}
				if err != tt.wantErr {
					t.Errorf("GetFileHash() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("GetFileHash() unexpected error = %v", err)
				return
			}

			if gotHash != tt.wantHash {
				t.Errorf("GetFileHash() = %v, want %v", gotHash, tt.wantHash)
			}
		})
	}
}

func TestGetFileHash_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a larger file (1MB)
	largeFile := filepath.Join(tmpDir, "large.bin")
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	os.WriteFile(largeFile, data, 0644)

	hash, err := GetFileHash(largeFile)
	if err != nil {
		t.Fatalf("GetFileHash() error = %v", err)
	}

	// Hash should be 64 hex characters (256 bits = 32 bytes = 64 hex chars)
	if len(hash) != 64 {
		t.Errorf("GetFileHash() hash length = %d, want 64", len(hash))
	}

	// Should be all lowercase hex
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("GetFileHash() hash contains invalid character: %c", c)
			break
		}
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
		{
			name:    "newline at end",
			input:   "hello\n",
			want:    "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03",
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

func TestHashPathFromHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{
			name: "typical sha256 hash",
			hash: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name: "another hash",
			hash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HashPathFromHash(tt.hash)

			// Should be in format "bucket-subbucket-hash"
			parts := strings.Split(result, "-")
			if len(parts) != 3 {
				t.Errorf("HashPathFromHash() = %q, expected 3 parts separated by -", result)
				return
			}

			// First part should be 0-999
			// Second part should be 5 digits (00000)
			if len(parts[1]) != 5 {
				t.Errorf("HashPathFromHash() subbucket = %q, expected 5 digits", parts[1])
			}

			// Third part should be the original hash
			if parts[2] != tt.hash {
				t.Errorf("HashPathFromHash() hash part = %q, want %q", parts[2], tt.hash)
			}
		})
	}
}

func TestHashPathFromHashWithSubbucket(t *testing.T) {
	hash := "abc123def456"

	tests := []struct {
		subbucket int
		wantEnd   string
	}{
		{0, "-00000-" + hash},
		{1, "-00001-" + hash},
		{99999, "-99999-" + hash},
	}

	for _, tt := range tests {
		result := HashPathFromHashWithSubbucket(hash, tt.subbucket)
		if !strings.HasSuffix(result, tt.wantEnd) {
			t.Errorf("HashPathFromHashWithSubbucket(%q, %d) = %q, want suffix %q",
				hash, tt.subbucket, result, tt.wantEnd)
		}
	}
}

func TestGetSubbucketFromHash(t *testing.T) {
	tests := []struct {
		hash string
	}{
		{"b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"},
		{"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"0000000000"},
		{"ffffffffff"},
	}

	for _, tt := range tests {
		result := GetSubbucketFromHash(tt.hash)

		// Should be in range 0-99999
		if result < 0 || result >= 100000 {
			t.Errorf("GetSubbucketFromHash(%q) = %d, want 0-99999", tt.hash, result)
		}
	}
}

func TestGetSubbucketFromHash_ShortHash(t *testing.T) {
	// Short hashes should return 0
	result := GetSubbucketFromHash("abc")
	if result != 0 {
		t.Errorf("GetSubbucketFromHash(short) = %d, want 0", result)
	}
}

func TestHexCharToInt(t *testing.T) {
	tests := []struct {
		input byte
		want  int
	}{
		{'0', 0}, {'1', 1}, {'9', 9},
		{'a', 10}, {'b', 11}, {'f', 15},
		{'A', 10}, {'B', 11}, {'F', 15},
		{'g', 0}, {'z', 0}, {' ', 0}, // Invalid chars return 0
	}

	for _, tt := range tests {
		got := hexCharToInt(tt.input)
		if got != tt.want {
			t.Errorf("hexCharToInt(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestHashFromHashPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantHash string
		wantErr  bool
	}{
		{
			name:     "valid path",
			path:     "742-00000-abc123def456",
			wantHash: "abc123def456",
			wantErr:  false,
		},
		{
			name:     "valid path with long hash",
			path:     "123-00001-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantErr:  false,
		},
		{
			name:    "invalid path - too few parts",
			path:    "742-abc123",
			wantErr: true,
		},
		{
			name:    "invalid path - no dashes",
			path:    "abc123def456",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HashFromHashPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashFromHashPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantHash {
				t.Errorf("HashFromHashPath() = %q, want %q", got, tt.wantHash)
			}
		})
	}
}

func TestWorkspacePrefixFromHashPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name:    "valid path",
			path:    "742-00000-abc123",
			want:    filepath.Join("742", "00000"),
			wantErr: false,
		},
		{
			name:    "invalid path",
			path:    "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := WorkspacePrefixFromHashPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("WorkspacePrefixFromHashPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("WorkspacePrefixFromHashPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZipPrefixFromHashPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name:    "valid path",
			path:    "742-00000-abc123",
			want:    "742-00000",
			wantErr: false,
		},
		{
			name:    "invalid path",
			path:    "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ZipPrefixFromHashPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ZipPrefixFromHashPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ZipPrefixFromHashPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
