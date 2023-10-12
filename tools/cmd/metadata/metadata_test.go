package main

import (
	"bytes"
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
		res.assertErr("found 2 issues in")
		res.assertOutput("", `
Method AService.MissingFromMetadata does not exist in metadata.yaml. Please add it.
Name in override_operations does not exist in operations or openapi_operations: GET /fake/{a_id}
`)
	})

	t.Run("valid", func(t *testing.T) {
		res := runTest(t, "testdata/validate_valid", "validate")
		res.assertOutput("", "")
		res.assertNoErr()
	})
}

func Test_canonizeCmd(t *testing.T) {
	res := runTest(t, "testdata/canonize", "canonize")
	res.assertOutput("", "")
	res.assertNoErr()
	res.checkGolden()
}

func assertEqualStrings(t *testing.T, want, got string) {
	t.Helper()
	diff := cmp.Diff(want, got)
	if diff != "" {
		t.Error(diff)
	}
}

func assertNilError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Error(err)
	}
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

func checkGoldenDir(t *testing.T, got string) {
	t.Helper()
	golden := filepath.Join("testdata", "golden", t.Name())
	if os.Getenv("UPDATE_GOLDEN") != "" {
		err := os.RemoveAll(golden)
		if err != nil {
			t.Error(err)
			return
		}
		err = copyDir(golden, got)
		if err != nil {
			t.Error(err)
			return
		}
		return
	}

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
		assertEqualStrings(t, string(wantContent), string(gotContent))
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

type testRun struct {
	t       *testing.T
	workDir string
	stdOut  bytes.Buffer
	stdErr  bytes.Buffer
	err     error
}

func (r testRun) checkGolden() {
	r.t.Helper()
	checkGoldenDir(r.t, r.workDir)
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
	}
	err := copyDir(res.workDir, filepath.FromSlash(srcDir))
	if err != nil {
		t.Error(err)
		return res
	}
	dv := kong.Vars{}
	for k, v := range defaultVars {
		dv[k] = v
	}
	dv["workingdir_default"] = srcDir
	args = append([]string{"--working-dir", srcDir}, args...)
	res.err = run(args, []kong.Option{kong.Writers(&res.stdOut, &res.stdErr), dv, helpVars})
	return res
}
