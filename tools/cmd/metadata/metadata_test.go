package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/google/go-cmp/cmp"
)

func Test_updateUrlsCmd(t *testing.T) {
	res := runTest(t, "testdata/updatedocs", "update-urls")
	res.assertOutput("", "")
	res.assertNoErr()
	res.checkGolden()
}

func Test_validateCmd(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		res := runTest(t, "testdata/validate_invalid", "validate")
		res.assertErr("found 4 issues in")
		res.assertOutput("", `
Method AService.MissingFromMetadata does not exist in metadata.yaml. Please add it.
Method AService.Get has operation which is does not use the canonical name. You may be able to automatically fix this by running 'script/metadata.sh canonize': GET /a/{a_id_noncanonical}.
Name in override_operations does not exist in operations or openapi_operations: GET /a/{a_id_noncanonical2}
Name in override_operations does not exist in operations or openapi_operations: GET /fake/{a_id}
`)
		res.checkGolden()
	})

	t.Run("valid", func(t *testing.T) {
		res := runTest(t, "testdata/validate_valid", "validate")
		res.assertOutput("", "")
		res.assertNoErr()
		res.checkGolden()
	})
}

func Test_canonizeCmd(t *testing.T) {
	res := runTest(t, "testdata/canonize", "canonize")
	res.assertOutput("", "")
	res.assertNoErr()
	res.checkGolden()
}

func Test_formatCmd(t *testing.T) {
	res := runTest(t, "testdata/format", "format")
	res.assertOutput("", "")
	res.assertNoErr()
	res.checkGolden()
}

func updateGoldenDir(t *testing.T, origDir, resultDir, goldenDir string) {
	t.Helper()
	err := os.RemoveAll(goldenDir)
	assertNilError(t, err)
	err = filepath.WalkDir(resultDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		relName := strings.TrimPrefix(path, resultDir)
		origName := filepath.Join(origDir, relName)
		_, err = os.Stat(origName)
		if err != nil {
			if os.IsNotExist(err) {
				err = os.MkdirAll(filepath.Dir(filepath.Join(goldenDir, relName)), d.Type())
				return copyFile(path, filepath.Join(goldenDir, relName))
			}
			return err
		}
		resContent, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		origContent, err := os.ReadFile(origName)
		if err != nil {
			return err
		}
		if bytes.Equal(resContent, origContent) {
			return nil
		}
		return copyFile(path, filepath.Join(goldenDir, relName))
	})
}

func checkGoldenDir(t *testing.T, origDir, resultDir, goldenDir string) {
	t.Helper()
	if os.Getenv("UPDATE_GOLDEN") != "" {
		updateGoldenDir(t, origDir, resultDir, goldenDir)
		return
	}
	checked := map[string]bool{}
	_, err := os.Stat(goldenDir)
	if err == nil {
		assertNilError(t, filepath.Walk(goldenDir, func(wantPath string, info fs.FileInfo, err error) error {
			relPath := strings.TrimPrefix(wantPath, goldenDir)
			if err != nil || info.IsDir() {
				return err
			}
			assertEqualFiles(t, wantPath, filepath.Join(resultDir, relPath))
			checked[relPath] = true
			return nil
		}))
	}
	assertNilError(t, filepath.Walk(origDir, func(wantPath string, info fs.FileInfo, err error) error {
		relPath := strings.TrimPrefix(wantPath, origDir)
		if err != nil || info.IsDir() || checked[relPath] {
			return err
		}
		assertEqualFiles(t, wantPath, filepath.Join(resultDir, relPath))
		checked[relPath] = true
		return nil
	}))
	assertNilError(t, filepath.Walk(resultDir, func(resultPath string, info fs.FileInfo, err error) error {
		relPath := strings.TrimPrefix(resultPath, resultDir)
		if err != nil || info.IsDir() || checked[relPath] {
			return err
		}
		return fmt.Errorf("file %q not found in golden dir", resultPath)
	}))
}

func copyDir(dst, src string) error {
	dst, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	return filepath.Walk(src, func(srcPath string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		dstPath := filepath.Join(dst, strings.TrimPrefix(srcPath, src))
		err = copyFile(srcPath, dstPath)
		return err
	})
}

func copyFile(src, dst string) (errOut error) {
	srcDirStat, err := os.Stat(filepath.Dir(src))
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Dir(dst), srcDirStat.Mode())
	if err != nil {
		return err
	}
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		e := dstFile.Close()
		if errOut == nil {
			errOut = e
		}
	}()
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		e := srcFile.Close()
		if errOut == nil {
			errOut = e
		}
	}()
	_, err = io.Copy(dstFile, srcFile)
	return err
}

type testRun struct {
	t       *testing.T
	workDir string
	srcDir  string
	stdOut  bytes.Buffer
	stdErr  bytes.Buffer
	err     error
}

func (r testRun) checkGolden() {
	r.t.Helper()
	checkGoldenDir(r.t, r.srcDir, r.workDir, filepath.Join("testdata", "golden", r.t.Name()))
}

func (r testRun) assertOutput(stdout, stderr string) {
	r.t.Helper()
	assertEqualStrings(r.t, strings.TrimSpace(stdout), strings.TrimSpace(r.stdOut.String()))
	assertEqualStrings(r.t, strings.TrimSpace(stderr), strings.TrimSpace(r.stdErr.String()))
}

func (r testRun) assertNoErr() {
	r.t.Helper()
	assertNilError(r.t, r.err)
}

func (r testRun) assertErr(want string) {
	r.t.Helper()
	if want == "" {
		assertError(r.t, r.err)
		return
	}
	assertErrorContains(r.t, want, r.err)
}

func runTest(t *testing.T, srcDir string, args ...string) testRun {
	t.Helper()
	res := testRun{
		t:       t,
		workDir: t.TempDir(),
		srcDir:  srcDir,
	}
	err := copyDir(res.workDir, filepath.FromSlash(srcDir))
	if err != nil {
		t.Error(err)
		return res
	}
	defaultVars["workingdir_default"] = res.workDir
	res.err = run(args, []kong.Option{kong.Writers(&res.stdOut, &res.stdErr), defaultVars, helpVars})
	return res
}

func assertEqualStrings(t *testing.T, want, got string) {
	t.Helper()
	diff := cmp.Diff(want, got)
	if diff != "" {
		t.Error(diff)
	}
}

func assertEqualFiles(t *testing.T, want, got string) {
	t.Helper()
	wantBytes, err := os.ReadFile(want)
	if !assertNilError(t, err) {
		return
	}
	gotBytes, err := os.ReadFile(got)
	if !assertNilError(t, err) {
		return
	}
	if bytes.Equal(wantBytes, gotBytes) {
		return
	}
	diff := cmp.Diff(string(wantBytes), string(gotBytes))
	t.Errorf("files %q and %q differ: %s", want, got, diff)
}

func assertNilError(t *testing.T, err error) bool {
	t.Helper()
	if err != nil {
		t.Error(err)
		return false
	}
	return true
}

func assertErrorContains(t *testing.T, want string, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected error")
		return
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("expected error to contain %q, got %q", want, err.Error())
	}
}

func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected error")
	}
}
