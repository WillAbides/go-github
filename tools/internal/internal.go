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
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	receiverName string
	methodName   string
	filename     string
	httpMethod   string
	helper       string
	urls         []string
}

func (m *serviceMethod) name() string {
	return fmt.Sprintf("%s.%s", m.receiverName, m.methodName)
}

func getServiceMethods(dir string) ([]*serviceMethod, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var serviceMethods []*serviceMethod
	for _, filename := range dirEntries {
		m, err := getServiceMethodsFromFile(filepath.Join(dir, filename.Name()))
		if err != nil {
			return nil, err
		}
		serviceMethods = append(serviceMethods, m...)
	}
	sort.Slice(serviceMethods, func(i, j int) bool {
		if serviceMethods[i].filename != serviceMethods[j].filename {
			return serviceMethods[i].filename < serviceMethods[j].filename
		}
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
		sm := serviceMethodFromNode(filename, n)
		if sm != nil {
			serviceMethods = append(serviceMethods, sm)
		}
		return true
	})
	return serviceMethods, nil
}

func serviceMethodFromNode(filename string, n ast.Node) *serviceMethod {
	decl, ok := n.(*ast.FuncDecl)
	if !ok {
		return nil
	}
	if decl.Recv == nil || len(decl.Recv.List) != 1 {
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
	receiverType := id.Name
	methodName := decl.Name.Name

	// We only want exported methods on exported types.
	// The receiver must either end with Service or be named Client.
	// The exception is github.go, which contains Client methods we want to skip.

	if !ast.IsExported(methodName) || !ast.IsExported(receiverType) {
		return nil
	}
	if receiverType != "Client" && !strings.HasSuffix(receiverType, "Service") {
		return nil
	}
	if receiverType == "Client" && filepath.Base(filename) == "github.go" {
		return nil
	}
	method := serviceMethod{
		receiverName: receiverType,
		methodName:   methodName,
		filename:     filename,
	}
	bd := &bodyData{receiverName: recv.Names[0].Name}
	err := bd.parseBody(decl.Body)
	if err != nil {
		return &method
	}
	method.httpMethod = bd.httpMethod
	method.urls = append(method.urls, bd.urlFormats...)
	if bd.helperMethod != "" {
		method.helper = receiverType + "." + bd.helperMethod
	}
	return &method
}

// bodyData contains information found in a BlockStmt.
type bodyData struct {
	receiverName string // receiver name of method to help identify helper methods.
	httpMethod   string
	urlVarName   string
	urlFormats   []string
	assignments  []lhsrhs
	helperMethod string // If populated, httpMethod lives in helperMethod.
}

func (b *bodyData) parseBody(body *ast.BlockStmt) error {
	// Find the variable used for the format string, its one-or-more values,
	// and the httpMethod used for the NewRequest.
	for _, stmt := range body.List {
		switch stmt := stmt.(type) {
		case *ast.AssignStmt:
			hm, uvn, hlp, asgn := processAssignStmt(b.receiverName, stmt)
			if b.httpMethod != "" && hm != "" && b.httpMethod != hm {
				return fmt.Errorf("found two httpMethod values: %q and %q", b.httpMethod, hm)
			}
			if hm != "" {
				b.httpMethod = hm
				// logf("parseBody: httpMethod=%v", b.httpMethod)
			}
			if hlp != "" {
				b.helperMethod = hlp
			}
			b.assignments = append(b.assignments, asgn...)

			rawFormat, err := strconv.Unquote(uvn)
			// we know it's a raw string literal if strconv.Unquote doesn't error
			if err == nil {
				b.urlFormats = append(b.urlFormats, rawFormat)
			}

			// logf("assignments=%#v", b.assignments)
			if b.urlVarName == "" && uvn != "" {
				b.urlVarName = uvn
				// logf("parseBody: urlVarName=%v", b.urlVarName)
				// By the time the urlVarName is found, all assignments should
				// have already taken place so that we can find the correct
				// ones and determine the urlFormats.
				for _, lr := range b.assignments {
					if lr.lhs == b.urlVarName {
						b.urlFormats = append(b.urlFormats, lr.rhs)
					}
				}
			}
		case *ast.DeclStmt:
		case *ast.DeferStmt:
		case *ast.ExprStmt:
		case *ast.IfStmt:
			if err := b.parseIf(stmt); err != nil {
				return err
			}
		case *ast.RangeStmt:
		case *ast.ReturnStmt: // Return Results
			if len(stmt.Results) > 0 {
				switch rslt0 := stmt.Results[0].(type) {
				case *ast.CallExpr:
					recv, funcName, args := processCallExpr(rslt0)
					// If the httpMethod has not been found at this point, but
					// this method is calling a helper function, then see if
					// any of its arguments match a previous assignment, then
					// record the urlFormat and remember the helper method.
					if b.httpMethod == "" && len(args) > 1 && recv == b.receiverName {
						if args[0] != "ctx" {
							return fmt.Errorf("expected helper function to get ctx as first arg: %#v, %#v", args, *b)
						}
						if len(b.assignments) == 0 && len(b.urlFormats) == 0 {
							b.urlFormats = append(b.urlFormats, strings.Trim(args[1], `"`))
							b.helperMethod = funcName
						} else {
							for _, lr := range b.assignments {
								if lr.lhs == args[1] { // Multiple matches are possible. Loop over all assignments.
									b.urlVarName = args[1]
									b.urlFormats = append(b.urlFormats, lr.rhs)
									b.helperMethod = funcName
								}
							}
						}
					}
				}
			}
		case *ast.SwitchStmt:
		default:
			return fmt.Errorf("unhandled stmt type: %T", stmt)
		}
	}

	return nil
}

func (b *bodyData) parseIf(stmt *ast.IfStmt) error {
	if err := b.parseBody(stmt.Body); err != nil {
		return err
	}
	if stmt.Else != nil {
		switch els := stmt.Else.(type) {
		case *ast.BlockStmt:
			if err := b.parseBody(els); err != nil {
				return err
			}
		case *ast.IfStmt:
			if err := b.parseIf(els); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unhandled else stmt type %T", els)
		}
	}

	return nil
}

// lhsrhs represents an assignment with a variable name on the left
// and a string on the right - used to find the URL format string.
type lhsrhs struct {
	lhs string
	rhs string
}

var (
	helperOverrides = map[string]func(arg string) (httpMethod, url string){
		"s.search": func(arg string) (httpMethod, url string) {
			return "GET", fmt.Sprintf("search/%v", arg)
		},
	}
)

func processAssignStmt(receiverName string, stmt *ast.AssignStmt) (httpMethod, urlVarName, helperMethod string, assignments []lhsrhs) {
	var lhs []string
	for _, expr := range stmt.Lhs {
		switch expr := expr.(type) {
		case *ast.Ident: // NamePos, Name, Obj
			lhs = append(lhs, expr.Name)
		case *ast.SelectorExpr: // X, Sel
		default:
			log.Fatalf("unhandled AssignStmt Lhs type: %T", expr)
		}
	}

	for i, expr := range stmt.Rhs {
		switch expr := expr.(type) {
		case *ast.BasicLit: // ValuePos, Kind, Value
			v := strings.Trim(expr.Value, `"`)
			if !strings.HasPrefix(v, "?") { // Hack to remove "?recursive=1"
				assignments = append(assignments, lhsrhs{lhs: lhs[i], rhs: v})
			}
		case *ast.BinaryExpr:
		case *ast.CallExpr: // Fun, Lparen, Args, Ellipsis, Rparen
			recv, funcName, args := processCallExpr(expr)
			switch funcName {
			case "addOptions":
				if v := strings.Trim(args[0], `"`); v != args[0] {
					assignments = append(assignments, lhsrhs{lhs: lhs[i], rhs: v})
					urlVarName = lhs[i]
				} else {
					urlVarName = args[0]
				}
			case "Sprintf":
				assignments = append(assignments, lhsrhs{lhs: lhs[i], rhs: strings.Trim(args[0], `"`)})
			case "NewRequest":
				httpMethod = strings.Trim(args[0], `"`)
				urlVarName = args[1]
			case "NewUploadRequest":
				httpMethod = "POST"
				urlVarName = args[0]
			case "roundTripWithOptionalFollowRedirect":
				httpMethod = "GET"
				urlVarName = args[1]
			}
			if recv == receiverName && len(args) > 1 && args[0] == "ctx" { // This might be a helper method.
				//if len(args) > 1 && args[0] == "ctx" { // This might be a helper method.
				fullName := fmt.Sprintf("%v.%v", recv, funcName)
				if fn, ok := helperOverrides[fullName]; ok {
					hm, url := fn(strings.Trim(args[1], `"`))
					httpMethod = hm
					urlVarName = "u" // arbitrary
					assignments = []lhsrhs{{lhs: urlVarName, rhs: url}}
				} else {
					urlVarName = args[1] // For this to work correctly, the URL must be the second arg to the helper method!
					helperMethod = funcName
				}
			}
		case *ast.CompositeLit: // Type, Lbrace, Elts, Rbrace, Incomplete
		case *ast.FuncLit:
		case *ast.SelectorExpr:
		case *ast.UnaryExpr: // OpPos, Op, X
		case *ast.TypeAssertExpr: // X, Lparen, Type, Rparen
		case *ast.Ident: // NamePos, Name, Obj
		default:
			log.Fatalf("unhandled AssignStmt Rhs type: %T", expr)
		}
	}

	return httpMethod, urlVarName, helperMethod, assignments
}

func processCallExpr(expr *ast.CallExpr) (recv, funcName string, args []string) {

	for _, arg := range expr.Args {
		switch arg := arg.(type) {
		case *ast.ArrayType:
		case *ast.BasicLit: // ValuePos, Kind, Value
			args = append(args, arg.Value) // Do not trim quotes here so as to identify it later as a string literal.
		case *ast.CallExpr: // Fun, Lparen, Args, Ellipsis, Rparen
			r, fn, as := processCallExpr(arg)
			if r == "fmt" && fn == "Sprintf" && len(as) > 0 { // Special case - return format string.
				args = append(args, as[0])
			}
		case *ast.CompositeLit:
		case *ast.Ident: // NamePos, Name, Obj
			args = append(args, arg.Name)
		case *ast.MapType:
		case *ast.SelectorExpr: // X, Sel
			x, ok := arg.X.(*ast.Ident)
			if ok { // special case
				name := fmt.Sprintf("%v.%v", x.Name, arg.Sel.Name)
				if strings.HasPrefix(name, "http.Method") {
					name = strings.ToUpper(strings.TrimPrefix(name, "http.Method"))
				}
				args = append(args, name)
			}
		case *ast.StarExpr:
		case *ast.StructType:
		case *ast.UnaryExpr: // OpPos, Op, X
			switch x := arg.X.(type) {
			case *ast.Ident:
				args = append(args, x.Name)
			case *ast.CompositeLit: // Type, Lbrace, Elts, Rbrace, Incomplete
			default:
				log.Fatalf("processCallExpr: unhandled UnaryExpr.X arg type: %T", arg.X)
			}
		default:
			log.Fatalf("processCallExpr: unhandled arg type: %T", arg)
		}
	}

	switch fun := expr.Fun.(type) {
	case *ast.Ident: // NamePos, Name, Obj
		funcName = fun.Name
	case *ast.SelectorExpr: // X, Sel
		funcName = fun.Sel.Name
		switch x := fun.X.(type) {
		case *ast.Ident: // NamePos, Name, Obj
			recv = x.Name
		case *ast.ParenExpr:
		case *ast.SelectorExpr: // X, Sel
			recv = x.Sel.Name
		case *ast.CallExpr: // Fun, LParen, Args, Ellipsis, RParen
		default:
			log.Fatalf("processCallExpr: unhandled X receiver type: %T, funcName=%q", x, funcName)
		}
	default:
		log.Fatalf("processCallExpr: unhandled Fun: %T", expr.Fun)
	}

	return recv, funcName, args
}
