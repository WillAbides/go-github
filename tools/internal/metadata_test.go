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

func setupGoldenTest(t *testing.T, srcDir string) (workDir string, check func()) {
	t.Helper()
	workDir = t.TempDir()
	err := copyDir(workDir, srcDir)
	require.NoError(t, err)
	return workDir, func() {
		checkGoldenDir(t, workDir)
	}
}

//func TestUpdateDocs(t *testing.T) {
//	workDir, check := setupGoldenTest(t, filepath.FromSlash("testdata/updatedocs"))
//	meta, err := LoadMetadataFile(filepath.Join(workDir, "metadata.yaml"))
//	require.NoError(t, err)
//	err = UpdateDocLinks(meta, workDir)
//	require.NoError(t, err)
//	check()
//}
//
//func TestMetadata_CanonizeMethodOperations(t *testing.T) {
//	workDir, check := setupGoldenTest(t, filepath.FromSlash("testdata/canonize"))
//	metafile := filepath.Join(workDir, "metadata.yaml")
//	meta, err := LoadMetadataFile(metafile)
//	require.NoError(t, err)
//	err = meta.CanonizeMethodOperations()
//	require.NoError(t, err)
//	err = meta.SaveFile(metafile)
//	require.NoError(t, err)
//	check()
//}
