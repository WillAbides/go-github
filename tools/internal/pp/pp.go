package pp

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"strconv"
	"strings"
)

const (
	stdURL = "docs.github.com"
)

var (
	helperOverrides = map[string]overrideFunc{
		"s.search": func(arg string) (httpMethod, url string) {
			return "GET", fmt.Sprintf("search/%v", arg)
		},
	}

	// skipMethods holds methods which are skipped because they do not have GitHub v3
	// API URLs or are otherwise problematic in parsing, discovering, and/or fixing.
	skipMethods = map[string]bool{
		"RepositoriesService.DownloadContents":         true,
		"RepositoriesService.DownloadContentsWithMeta": true,
		"RepositoriesService.Subscribe":                true,
		"RepositoriesService.Unsubscribe":              true,
	}
)

type overrideFunc func(arg string) (httpMethod, url string)

type servicesMap map[string]*Service
type endpointsMap map[string]*Endpoint

// Service represents a go-github service.
type Service struct {
	serviceName string
}

// Endpoint represents an API endpoint in this repo.
type Endpoint struct {
	endpointName string
	filename     string
	serviceName  string
	urlFormats   []string
	httpMethod   string
	helperMethod string // If populated, httpMethod lives in helperMethod.

	enterpriseRefLines []*ast.Comment
	stdRefLines        []*ast.Comment
	endpointComments   []*ast.Comment
}

// astFileIterator iterates over all files in an ast.Package.
type astFileIterator interface {
	// Finds the position of a token.
	Position(token.Pos) token.Position
	// Reset resets the iterator.
	Reset()
	// Next returns the next filenameAstFilePair pair or nil if done.
	Next() *filenameAstFilePair
}

type filenameAstFilePair struct {
	filename string
	astFile  *ast.File
}

func findAllServices(pkgs map[string]*ast.Package) servicesMap {
	services := servicesMap{}
	for _, pkg := range pkgs {
		for filename, f := range pkg.Files {
			if !strings.HasSuffix(filename, "github.go") {
				continue
			}

			logf("Step 1 - Processing %v ...", filename)
			if err := findClientServices(f, services); err != nil {
				log.Fatal(err)
			}
		}
	}
	return services
}

// findClientServices finds all go-github services from the Client struct.
func findClientServices(f *ast.File, services servicesMap) error {
	for _, decl := range f.Decls {
		switch decl := decl.(type) {
		case *ast.GenDecl:
			if decl.Tok != token.TYPE || len(decl.Specs) != 1 {
				continue
			}
			ts, ok := decl.Specs[0].(*ast.TypeSpec)
			if !ok || decl.Doc == nil || ts.Name == nil || ts.Type == nil || ts.Name.Name != "Client" {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil || len(st.Fields.List) == 0 {
				continue
			}

			for _, field := range st.Fields.List {
				se, ok := field.Type.(*ast.StarExpr)
				if !ok || se.X == nil || len(field.Names) != 1 {
					continue
				}
				id, ok := se.X.(*ast.Ident)
				if !ok {
					continue
				}
				name := id.Name
				if !strings.HasSuffix(name, "Service") {
					continue
				}

				services[name] = &Service{serviceName: name}
			}

			return nil // Found all services in Client struct.
		}
	}

	return fmt.Errorf("unable to find Client struct in github.go")
}

func findAllServiceEndpoints(iter astFileIterator, services servicesMap) (endpointsMap, error) {
	endpoints := endpointsMap{}
	iter.Reset()
	var errs []string // Collect all the errors and return in a big batch.
	for next := iter.Next(); next != nil; next = iter.Next() {
		filename, f := next.filename, next.astFile
		if strings.HasSuffix(filename, "github.go") {
			continue
		}

		//if *debugFile != "" && !strings.Contains(filename, *debugFile) {
		//	continue
		//}

		logf("Step 2 - Processing %v ...", filename)
		if err := processAST(filename, f, services, endpoints, iter); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "\n"))
	}

	return endpoints, nil
}

func processAST(filename string, f *ast.File, services servicesMap, endpoints endpointsMap, iter astFileIterator) error {
	var errs []string

	for _, decl := range f.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl: // Doc, Recv, Name, Type, Body
			if decl.Recv == nil || len(decl.Recv.List) != 1 || decl.Name == nil || decl.Body == nil {
				continue
			}

			recv := decl.Recv.List[0]
			se, ok := recv.Type.(*ast.StarExpr) // Star, X
			if !ok || se.X == nil || len(recv.Names) != 1 {
				if decl.Name.Name != "String" && decl.Name.Name != "Equal" && decl.Name.Name != "IsPullRequest" {
					pos := iter.Position(recv.Pos())
					if id, ok := recv.Type.(*ast.Ident); ok {
						pos = iter.Position(id.Pos())
					}
					errs = append(errs, fmt.Sprintf("%v:%v:%v: method %v does not use a pointer receiver and needs fixing!", pos.Filename, pos.Line, pos.Column, decl.Name))
				}
				continue
			}
			recvType, ok := se.X.(*ast.Ident) // NamePos, Name, Obj
			if !ok {
				return fmt.Errorf("unhandled se.X = %T", se.X)
			}
			serviceName := recvType.Name
			if _, ok := services[serviceName]; !ok {
				continue
			}
			endpointName := decl.Name.Name
			fullName := fmt.Sprintf("%v.%v", serviceName, endpointName)
			if skipMethods[fullName] {
				logf("skipping %v", fullName)
				continue
			}

			receiverName := recv.Names[0].Name

			logf("\n\nast.FuncDecl: %#v", *decl)       // Doc, Recv, Name, Type, Body
			logf("ast.FuncDecl.Name: %#v", *decl.Name) // NamePos, Name, Obj(nil)
			// logf("ast.FuncDecl.Recv: %#v", *decl.Recv)  // Opening, List, Closing
			logf("ast.FuncDecl.Recv.List[0]: %#v", *recv) // Doc, Names, Type, Tag, Comment
			// for i, name := range decl.Recv.List[0].Names {
			// 	logf("recv.name[%v] = %v", i, name.Name)
			// }
			logf("recvType = %#v", recvType)
			var enterpriseRefLines []*ast.Comment
			var stdRefLines []*ast.Comment
			var endpointComments []*ast.Comment
			if decl.Doc != nil {
				endpointComments = decl.Doc.List
				for i, comment := range decl.Doc.List {
					logf("doc.comment[%v] = %#v", i, *comment)
					// if strings.Contains(comment.Text, enterpriseURL) {
					// 	enterpriseRefLines = append(enterpriseRefLines, comment)
					// } else
					if strings.Contains(comment.Text, stdURL) {
						stdRefLines = append(stdRefLines, comment)
					}
				}
				logf("%v comment lines, %v enterprise URLs, %v standard URLs", len(decl.Doc.List), len(enterpriseRefLines), len(stdRefLines))
			}

			bd := &bodyData{receiverName: receiverName}
			if err := bd.parseBody(decl.Body); err != nil { // Lbrace, List, Rbrace
				return fmt.Errorf("parseBody: %v", err)
			}

			ep := &Endpoint{
				endpointName:       endpointName,
				filename:           filename,
				serviceName:        serviceName,
				urlFormats:         bd.urlFormats,
				httpMethod:         bd.httpMethod,
				helperMethod:       bd.helperMethod,
				enterpriseRefLines: enterpriseRefLines,
				stdRefLines:        stdRefLines,
				endpointComments:   endpointComments,
			}
			endpoints[fullName] = ep
			logf("endpoints[%q] = %#v", fullName, endpoints[fullName])
			if ep.httpMethod == "" && (ep.helperMethod == "" || len(ep.urlFormats) == 0) {
				// only error for exported methods
				if ast.IsExported(endpointName) {
					return fmt.Errorf("filename=%q, endpoint=%q: could not find body info: %#v", filename, fullName, *ep)
				}
			}
		case *ast.GenDecl:
		default:
			return fmt.Errorf("unhandled decl type: %T", decl)
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}

	return nil
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
	logf("body=%#v", *body)

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
						logf("found urlFormat: %v", lr.rhs)
					}
				}
			}
		case *ast.DeclStmt:
			logf("*ast.DeclStmt: %#v", *stmt)
		case *ast.DeferStmt:
			logf("*ast.DeferStmt: %#v", *stmt)
		case *ast.ExprStmt:
			logf("*ast.ExprStmt: %#v", *stmt)
		case *ast.IfStmt:
			if err := b.parseIf(stmt); err != nil {
				return err
			}
		case *ast.RangeStmt:
			logf("*ast.RangeStmt: %#v", *stmt)
		case *ast.ReturnStmt: // Return Results
			logf("*ast.ReturnStmt: %#v", *stmt)
			if len(stmt.Results) > 0 {
				switch rslt0 := stmt.Results[0].(type) {
				case *ast.CallExpr:
					recv, funcName, args := processCallExpr(rslt0)
					logf("return CallExpr: recv=%q, funcName=%q, args=%#v", recv, funcName, args)
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
							switch b.helperMethod {
							//case "deleteReaction":
							//	b.httpMethod = "DELETE"
							default:
								logf("WARNING: helper method %q not found", b.helperMethod)
								//fmt.Printf("WARNING: helper method %q not found\n", b.helperMethod)
							}
							logf("found urlFormat: %v and helper method: %v, httpMethod: %v", b.urlFormats[0], b.helperMethod, b.httpMethod)
							//fmt.Printf("found urlFormat: %v and helper method: %v, httpMethod: %v\n", b.urlFormats[0], b.helperMethod, b.httpMethod)
						} else {
							for _, lr := range b.assignments {
								if lr.lhs == args[1] { // Multiple matches are possible. Loop over all assignments.
									b.urlVarName = args[1]
									b.urlFormats = append(b.urlFormats, lr.rhs)
									b.helperMethod = funcName
									logf("found urlFormat: %v and helper method: %v, httpMethod: %v", lr.rhs, b.helperMethod, b.httpMethod)
								}
							}
						}
					}
				default:
					logf("WARNING: stmt.Results[0] unhandled type = %T = %#v", stmt.Results[0], stmt.Results[0])
				}
			}
		case *ast.SwitchStmt:
			logf("*ast.SwitchStmt: %#v", *stmt)
		default:
			return fmt.Errorf("unhandled stmt type: %T", stmt)
		}
	}
	logf("parseBody: assignments=%#v", b.assignments)

	return nil
}

func (b *bodyData) parseIf(stmt *ast.IfStmt) error {
	logf("parseIf: *ast.IfStmt: %#v", *stmt)
	if err := b.parseBody(stmt.Body); err != nil {
		return err
	}
	logf("parseIf: if body: b=%#v", *b)
	if stmt.Else != nil {
		switch els := stmt.Else.(type) {
		case *ast.BlockStmt:
			if err := b.parseBody(els); err != nil {
				return err
			}
			logf("parseIf: if else: b=%#v", *b)
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

func processCallExpr(expr *ast.CallExpr) (recv, funcName string, args []string) {
	logf("*ast.CallExpr: %#v", *expr)

	for _, arg := range expr.Args {
		switch arg := arg.(type) {
		case *ast.ArrayType:
			logf("processCallExpr: *ast.ArrayType: %#v", arg)
		case *ast.BasicLit: // ValuePos, Kind, Value
			args = append(args, arg.Value) // Do not trim quotes here so as to identify it later as a string literal.
		case *ast.CallExpr: // Fun, Lparen, Args, Ellipsis, Rparen
			logf("processCallExpr: *ast.CallExpr: %#v", arg)
			r, fn, as := processCallExpr(arg)
			if r == "fmt" && fn == "Sprintf" && len(as) > 0 { // Special case - return format string.
				args = append(args, as[0])
			}
		case *ast.CompositeLit:
			logf("processCallExpr: *ast.CompositeLit: %#v", arg) // Type, Lbrace, Elts, Rbrace, Incomplete
		case *ast.Ident: // NamePos, Name, Obj
			args = append(args, arg.Name)
		case *ast.MapType:
			logf("processCallExpr: *ast.MapType: %#v", arg)
		case *ast.SelectorExpr: // X, Sel
			logf("processCallExpr: *ast.SelectorExpr: %#v", arg)
			x, ok := arg.X.(*ast.Ident)
			if ok { // special case
				name := fmt.Sprintf("%v.%v", x.Name, arg.Sel.Name)
				if strings.HasPrefix(name, "http.Method") {
					name = strings.ToUpper(strings.TrimPrefix(name, "http.Method"))
				}
				args = append(args, name)
			}
		case *ast.StarExpr:
			logf("processCallExpr: *ast.StarExpr: %#v", arg)
		case *ast.StructType:
			logf("processCallExpr: *ast.StructType: %#v", arg)
		case *ast.UnaryExpr: // OpPos, Op, X
			switch x := arg.X.(type) {
			case *ast.Ident:
				args = append(args, x.Name)
			case *ast.CompositeLit: // Type, Lbrace, Elts, Rbrace, Incomplete
				logf("processCallExpr: *ast.CompositeLit: %#v", x)
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
			logf("processCallExpr: X recv *ast.Ident=%#v", x)
			recv = x.Name
		case *ast.ParenExpr:
			logf("processCallExpr: X recv *ast.ParenExpr: %#v", x)
		case *ast.SelectorExpr: // X, Sel
			logf("processCallExpr: X recv *ast.SelectorExpr: %#v", x.Sel)
			recv = x.Sel.Name
		case *ast.CallExpr: // Fun, LParen, Args, Ellipsis, RParen
			logf("processCallExpr: X recv *ast.CallExpr: %#v", x)
		default:
			log.Fatalf("processCallExpr: unhandled X receiver type: %T, funcName=%q", x, funcName)
		}
	default:
		log.Fatalf("processCallExpr: unhandled Fun: %T", expr.Fun)
	}

	return recv, funcName, args
}

func logf(fmt string, args ...interface{}) {
	//log.Printf(fmt, args...)
}

// lhsrhs represents an assignment with a variable name on the left
// and a string on the right - used to find the URL format string.
type lhsrhs struct {
	lhs string
	rhs string
}

func processAssignStmt(receiverName string, stmt *ast.AssignStmt) (httpMethod, urlVarName, helperMethod string, assignments []lhsrhs) {
	logf("*ast.AssignStmt: %#v", *stmt) // Lhs, TokPos, Tok, Rhs
	var lhs []string
	for _, expr := range stmt.Lhs {
		switch expr := expr.(type) {
		case *ast.Ident: // NamePos, Name, Obj
			logf("processAssignStmt: *ast.Ident: %#v", expr)
			lhs = append(lhs, expr.Name)
		case *ast.SelectorExpr: // X, Sel
			logf("processAssignStmt: *ast.SelectorExpr: %#v", expr)
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
			logf("processAssignStmt: *ast.BinaryExpr: %#v", *expr)
		case *ast.CallExpr: // Fun, Lparen, Args, Ellipsis, Rparen
			recv, funcName, args := processCallExpr(expr)
			logf("processAssignStmt: CallExpr: recv=%q, funcName=%q, args=%#v", recv, funcName, args)
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
			default:
				logf("WARNING: processAssignStmt: unhandled CallExpr: recv=%q, funcName=%q, args=%#v", recv, funcName, args)
			}
			if recv == receiverName && len(args) > 1 && args[0] == "ctx" { // This might be a helper method.
				fullName := fmt.Sprintf("%v.%v", recv, funcName)
				logf("checking for override: fullName=%v", fullName)
				if fn, ok := helperOverrides[fullName]; ok {
					logf("found helperOverride for %v", fullName)
					hm, url := fn(strings.Trim(args[1], `"`))
					httpMethod = hm
					urlVarName = "u" // arbitrary
					assignments = []lhsrhs{{lhs: urlVarName, rhs: url}}
				} else {
					urlVarName = args[1] // For this to work correctly, the URL must be the second arg to the helper method!
					helperMethod = funcName
					logf("found possible helper method: funcName=%v, urlVarName=%v", funcName, urlVarName)
				}
			}
		case *ast.CompositeLit: // Type, Lbrace, Elts, Rbrace, Incomplete
			logf("processAssignStmt: *ast.CompositeLit: %#v", *expr)
		case *ast.FuncLit:
			logf("processAssignStmt: *ast.FuncLit: %#v", *expr)
		case *ast.SelectorExpr:
			logf("processAssignStmt: *ast.SelectorExpr: %#v", *expr)
		case *ast.UnaryExpr: // OpPos, Op, X
			logf("processAssignStmt: *ast.UnaryExpr: %#v", *expr)
		case *ast.TypeAssertExpr: // X, Lparen, Type, Rparen
			logf("processAssignStmt: *ast.TypeAssertExpr: %#v", *expr)
		case *ast.Ident: // NamePos, Name, Obj
			logf("processAssignStmt: *ast.Ident: %#v", *expr)
		default:
			log.Fatalf("unhandled AssignStmt Rhs type: %T", expr)
		}
	}
	logf("urlVarName=%v, assignments=%#v", urlVarName, assignments)

	return httpMethod, urlVarName, helperMethod, assignments
}

func resolveHelpers(endpoints endpointsMap) error {
	logf("Step 3 - resolving helpers and cache docs ...")
	usedHelpers := map[string]bool{}
	endpointsByFilename := map[string][]*Endpoint{}
	for k, v := range endpoints {
		if _, ok := endpointsByFilename[v.filename]; !ok {
			endpointsByFilename[v.filename] = []*Endpoint{}
		}
		endpointsByFilename[v.filename] = append(endpointsByFilename[v.filename], v)

		if v.httpMethod == "" && v.helperMethod != "" {
			fullName := fmt.Sprintf("%v.%v", v.serviceName, v.helperMethod)
			hm, ok := endpoints[fullName]
			if !ok {
				return fmt.Errorf("unable to find helper method %q for %q", fullName, k)
			}
			if hm.httpMethod == "" {
				return fmt.Errorf("helper method %q for %q has empty httpMethod: %#v", fullName, k, hm)
			}
			v.httpMethod = hm.httpMethod
			usedHelpers[fullName] = true
		}
	}

	return nil
}
