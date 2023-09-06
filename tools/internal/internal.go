package internal

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// ProjRootDir returns the go-github root directory that contains dir.
// Returns an error if dir is not in a go-github root.
func ProjRootDir(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	ok, err := isGoGithubRoot(dir)
	if err != nil {
		return "", err
	}
	if ok {
		return dir, nil
	}
	parent := filepath.Dir(dir)
	if parent == dir {
		return "", fmt.Errorf("not in a go-github root")
	}
	return ProjRootDir(parent)
}

func isGoGithubRoot(dir string) (bool, error) {
	filename := filepath.Join(dir, "go.mod")
	b, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	mf, err := modfile.ParseLax(filename, b, nil)
	if err != nil {
		// an invalid go.mod file is not a go-github root, so we don't care about the error
		return false, nil
	}
	if mf.Module == nil {
		return false, nil
	}
	// This gets rid of the /vN suffix if it exists.
	base, _, ok := module.SplitPathVersion(mf.Module.Mod.Path)
	if !ok {
		return false, nil
	}
	return base == "github.com/google/go-github", nil
}

func ExitErr(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
