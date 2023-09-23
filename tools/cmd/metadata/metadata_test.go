package main

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

func Test_updateUrlsCmd(t *testing.T) {
	tmpDir := t.TempDir()
	err := copyDir(tmpDir, filepath.FromSlash("testdata/updatedocs"))
	require.NoError(t, err)
	cmd := rootCmd{
		Filename: filepath.Join(tmpDir, "metadata.yaml"),
		GithubDir: filepath.Join(tmpDir, "github"),
	}
	err = cmd.UpdateUrls.Run(&cmd)
	require.NoError(t, err)
	checkGoldenDir(t, tmpDir)
}

func Test_validateCmd(t *testing.T) {
	srcDir := filepath.FromSlash("testdata/updatedocs")
	cmd := rootCmd{
		Filename: filepath.Join(srcDir, "metadata.yaml"),
		GithubDir: filepath.Join(srcDir, "github"),
	}
	err := cmd.Validate.Run(&cmd)
	require.NoError(t, err)
}
