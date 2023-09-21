// Copyright 2023 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

// isGoGithubRoot determines whether dir is the repo root of go-github. It does
// this by checking whether go.mod exists and contains a module directive with
// the path "github.com/google/go-github/vNN".
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

type serviceMethod struct {
	receiverType string
	methodName   string
}

func (m *serviceMethod) name() string {
	return fmt.Sprintf("%s.%s", m.receiverType, m.methodName)
}

func getServiceMethods(dir string) ([]*serviceMethod, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var serviceMethods []*serviceMethod
	for _, filename := range dirEntries {
		var sm []*serviceMethod
		sm, err = getServiceMethodsFromFile(filepath.Join(dir, filename.Name()))
		if err != nil {
			return nil, err
		}
		serviceMethods = append(serviceMethods, sm...)
	}
	sort.Slice(serviceMethods, func(i, j int) bool {
		return serviceMethods[i].name() < serviceMethods[j].name()
	})
	return serviceMethods, nil
}

// getServiceMethodsFromFile returns the service methods in filename.
func getServiceMethodsFromFile(filename string) ([]*serviceMethod, error) {
	if !strings.HasSuffix(filename, ".go") ||
		strings.HasSuffix(filename, "_test.go") {
		return nil, nil
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// Only look at the github package
	if f.Name.Name != "github" {
		return nil, nil
	}
	var serviceMethods []*serviceMethod
	ast.Inspect(f, func(n ast.Node) bool {
		sm := serviceMethodFromNode(n)
		if sm == nil {
			return true
		}
		serviceMethods = append(serviceMethods, sm)
		return false
	})
	return serviceMethods, nil
}

func serviceMethodFromNode(node ast.Node) *serviceMethod {
	decl, ok := node.(*ast.FuncDecl)
	if !ok || decl.Recv == nil || len(decl.Recv.List) != 1 {
		return nil
	}
	recv := decl.Recv.List[0]
	se, ok := recv.Type.(*ast.StarExpr)
	if !ok {
		return nil
	}
	id, ok := se.X.(*ast.Ident)
	if !ok {
		return nil
	}

	// We only want exported methods on exported types where the type name ends in "Service".
	if !id.IsExported() || !decl.Name.IsExported() || !strings.HasSuffix(id.Name, "Service") {
		return nil
	}

	return &serviceMethod{
		receiverType: id.Name,
		methodName:   decl.Name.Name,
	}
}
