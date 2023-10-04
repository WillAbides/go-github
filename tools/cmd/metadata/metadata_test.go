package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
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

func runTest(args ...string) (stdout, stderr []byte, _ error) {
	var so, se bytes.Buffer
	err := run(args, []kong.Option{kong.Writers(&so, &se), helpVars})
	return so.Bytes(), se.Bytes(), err
}

func Test_updateUrlsCmd(t *testing.T) {
	tmpDir := t.TempDir()
	err := copyDir(tmpDir, filepath.FromSlash("testdata/updatedocs"))
	require.NoError(t, err)
	stdout, stderr, err := runTest(
		"update-urls",
		"--github-dir", filepath.Join(tmpDir, "github"),
		"--filename", filepath.Join(tmpDir, "metadata.yaml"),
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
}

func Test_validateCmd(t *testing.T) {
	srcDir := filepath.FromSlash("testdata/updatedocs")
	stdout, stderr, err := runTest("validate", "--filename", filepath.Join(srcDir, "metadata.yaml"), "--github-dir", filepath.Join(srcDir, "github"))
	fmt.Println(err)
	fmt.Printf("%T\n", err)
	assert.ErrorContains(t, err, "found 1")

	fmt.Println("stderr")
	fmt.Println(string(stderr))
	fmt.Println("end stderr")
	_ = stderr
	_ = stdout

}
