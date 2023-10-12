package internal

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func copyDir(dst, src string) error {
	dst, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	return filepath.Walk(src, func(srcPath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, strings.TrimPrefix(srcPath, src))
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		srcContent, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, srcContent, info.Mode())
	})
}

func checkGoldenDir(t *testing.T, got string) bool {
	t.Helper()
	golden := filepath.Join("testdata", "golden", t.Name())
	if os.Getenv("UPDATE_GOLDEN") != "" {
		err := os.RemoveAll(golden)
		if !assert.NoError(t, err) {
			return false
		}
		err = copyDir(golden, got)
		return assert.NoError(t, err)
	}

	failed := false
	err := filepath.Walk(golden, func(wantPath string, info fs.FileInfo, err error) error {
		t.Helper()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		wantContent, err := os.ReadFile(wantPath)
		if err != nil {
			return err
		}
		gotPath := filepath.Join(got, strings.TrimPrefix(wantPath, golden))
		gotContent, err := os.ReadFile(gotPath)
		if err != nil {
			return err
		}
		if !assert.Equal(t, string(wantContent), string(gotContent)) {
			failed = true
		}
		return nil
	})
	if err != nil {
		t.Error(err)
		return false
	}
	return !failed
}

func TestUpdateDocs(t *testing.T) {
	srcDir := filepath.FromSlash("testdata/updatedocs")
	tmpDir := t.TempDir()
	err := copyDir(tmpDir, srcDir)
	require.NoError(t, err)
	meta, err := LoadMetadataFile(filepath.Join(tmpDir, "metadata.yaml"))
	require.NoError(t, err)
	err = UpdateDocLinks(meta, tmpDir)
	require.NoError(t, err)
	checkGoldenDir(t, tmpDir)
}
