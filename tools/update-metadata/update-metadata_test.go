package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/google/go-github/tools/internal"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func updateFile(filename string) error {
	fset := token.NewFileSet()
	f, err := decorator.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	dst.Inspect(f, func(n dst.Node) bool {
		d, ok := n.(*dst.FuncDecl)
		if !ok ||
			!d.Name.IsExported() ||
			d.Recv == nil {
			return true
		}
		if len(d.Decs.Start.All()) == 0 {
			d.Decs.Start.Append("// TODO: document exported function")
		}
		return true
	})
	var buf bytes.Buffer
	err = decorator.Fprint(&buf, f)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, buf.Bytes(), 0644)
}

var docLineRE = regexp.MustCompile(`(?i)\s*(//|\*\s*)?GitHub\s+API\s+docs:`)

func updateFile2(fset *token.FileSet, af *ast.File, m *internal.Metadata) (errOut error) {
	filename := fset.Position(af.Pos()).Filename
	df, err := decorator.DecorateFile(fset, af)
	if err != nil {
		return err
	}
	dst.Inspect(df, func(n dst.Node) bool {
		d, ok := n.(*dst.FuncDecl)
		if !ok ||
			!d.Name.IsExported() ||
			d.Recv == nil {
			return true
		}
		methodName := d.Name.Name
		receiverType := ""
		switch x := d.Recv.List[0].Type.(type) {
		case *dst.Ident:
			receiverType = x.Name
		case *dst.StarExpr:
			receiverType = x.X.(*dst.Ident).Name
		}

		var starts []string
		for _, s := range d.Decs.Start.All() {
			if !docLineRE.MatchString(s) {
				starts = append(starts, s)
			}
		}
		docLinks := m.DocLinksForMethod(strings.Join([]string{receiverType, methodName}, "."))
		if len(docLinks) > 0 {
			starts = append(starts, "//")
			for _, dl := range docLinks {
				starts = append(starts, fmt.Sprintf("// GitHub API docs: %s", dl))
			}
		}
		d.Decs.Start.Replace(starts...)
		return true
	})
	outFile, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		e := outFile.Close()
		if errOut == nil {
			errOut = e
		}
	}()
	return decorator.Fprint(outFile, df)
}

func TestOMG(t *testing.T) {
	gghRoot, err := internal.ProjRootDir(".")
	require.NoError(t, err)
	metadataFilename := filepath.Join(gghRoot, "metadata.yaml")
	metadataFile := &internal.Metadata{}
	err = internal.LoadMetadataFile(metadataFilename, metadataFile)
	require.NoError(t, err)
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(
		fset,
		filepath.Join(gghRoot, "github"),
		func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") },
		parser.ParseComments,
	)
	require.NoError(t, err)

	ghPkg := pkgs["github"]
	var eg errgroup.Group
	for k := range ghPkg.Files {
		f := ghPkg.Files[k]
		eg.Go(func() error {
			return updateFile2(fset, f, metadataFile)
		})
	}
	err = eg.Wait()
	require.NoError(t, err)
}

func TestDave(t *testing.T) {
	gghRoot, err := internal.ProjRootDir(".")
	require.NoError(t, err)
	ghDir, err := os.ReadDir(filepath.Join(gghRoot, "github"))
	require.NoError(t, err)
	var sourceFiles []string
	for _, f := range ghDir {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".go") && !strings.HasSuffix(f.Name(), "_test.go") {
			sourceFiles = append(sourceFiles, filepath.Join(gghRoot, "github", f.Name()))
		}
	}

	var eg errgroup.Group
	for i := range sourceFiles {
		filename := sourceFiles[i]
		eg.Go(func() error {
			return updateFile(filename)
		})
	}
	err = eg.Wait()
	require.NoError(t, err)
}

func TestRedoc(t *testing.T) {
	gghRoot, err := internal.ProjRootDir(".")
	require.NoError(t, err)
	fset := token.NewFileSet()
	githubGo := filepath.Join(gghRoot, "github", "github.go")
	node, err := parser.ParseFile(fset, githubGo, nil, parser.ParseComments)
	require.NoError(t, err)
	//var comments []*ast.CommentGroup
	ast.Inspect(node, func(n ast.Node) bool {
		//c, ok := n.(*ast.CommentGroup)
		//if ok {
		//	comments = append(comments, c)
		//}
		fn, ok := n.(*ast.FuncDecl)
		if !ok || !fn.Name.IsExported() || fn.Doc.Text() != "" {
			return true
		}
		fmt.Printf("exported function declaration without documentation found on line %d: \n\t%s\n", fset.Position(fn.Pos()).Line, fn.Name.Name)

		cg := &ast.CommentGroup{
			List: []*ast.Comment{
				{
					Text:  "// TODO: document exported function",
					Slash: fn.Pos() - 1,
				},
			},
		}
		fn.Doc = cg
		//comments = append(comments, cg)
		//node.Comments = append(node.Comments, cg)
		return true
	})
	//node.Comments = comments
	var buf bytes.Buffer
	err = printer.Fprint(&buf, fset, node)
	require.NoError(t, err)
	err = os.WriteFile(githubGo, buf.Bytes(), 0644)
	require.NoError(t, err)
}

func TestUncovered(t *testing.T) {
	gghRoot, err := internal.ProjRootDir(".")
	require.NoError(t, err)
	metadataFilename := filepath.Join(gghRoot, "metadata.yaml")
	metadataFile := &internal.Metadata{}
	err = internal.LoadMetadataFile(metadataFilename, metadataFile)
	require.NoError(t, err)
	count := 0
	for _, op := range metadataFile.Operations {
		if slices.Contains(op.OpenAPIFiles, "descriptions/api.github.com/api.github.com.json") {
			continue
		}
		if len(op.GoMethods) == 0 {
			count++
			fmt.Println(op.Method(), op.EndpointURL(), op.DocumentationURL())
		}
	}
	fmt.Println(count)
}

func TestFooBar(t *testing.T) {
	gghRoot, err := internal.ProjRootDir(".")
	metadataFilename := filepath.Join(gghRoot, "metadata.yaml")
	metadataFile := &internal.Metadata{}
	err = internal.LoadMetadataFile(metadataFilename, metadataFile)
	require.NoError(t, err)
	githubDir := filepath.Join(gghRoot, "github")
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, githubDir, nil, parser.ParseComments)
	require.NoError(t, err)
	for k, pkg := range pkgs {
		fmt.Println(k)
		fmt.Printf("package %s\n", pkg.Name)
	}
	ghPkg, ok := pkgs["github"]
	require.True(t, ok)
	var serviceMethods []ServiceMethod
	for _, astFile := range ghPkg.Files {
		ast.Inspect(astFile, func(n ast.Node) bool {
			x, ok := n.(*ast.FuncDecl)
			if !ok {
				return true
			}
			if x.Recv == nil || len(x.Recv.List) != 1 {
				return true
			}
			se, ok := x.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				return true
			}
			id, ok := se.X.(*ast.Ident)
			if !ok {
				return true
			}
			if !strings.HasSuffix(id.Name, "Service") {
				return true
			}
			if !ast.IsExported(x.Name.Name) {
				return true
			}
			filename := fset.Position(x.Pos()).Filename
			if strings.HasSuffix(filename, "repos_hooks.go") {
				return true
			}
			serviceMethods = append(serviceMethods, ServiceMethod{
				ServiceName: id.Name,
				MethodName:  x.Name.Name,
				Filename:    filename,
				CodeComment: x.Doc.Text(),
			})
			return true
		})
	}
	noMatchCount := 0
	receivers := map[string]bool{}
	sort.Slice(serviceMethods, func(i, j int) bool {
		if serviceMethods[i].Filename != serviceMethods[j].Filename {
			return serviceMethods[i].Filename < serviceMethods[j].Filename
		}
		if serviceMethods[i].ServiceName != serviceMethods[j].ServiceName {
			return serviceMethods[i].ServiceName < serviceMethods[j].ServiceName
		}
		return serviceMethods[i].MethodName < serviceMethods[j].MethodName
	})
	for _, method := range serviceMethods {
		if method.ServiceName == "ActionsService" && strings.Contains(method.MethodName, "RequiredWorkflow") {
			continue
		}
		receivers[method.ServiceName] = true
		//docsLink := getDocsLink(method.CodeComment)
		//if docsLink == "" {
		//	continue
		//}
		docsLinks := getDocsLinks(method.CodeComment)
		for _, docsLink := range docsLinks {
			op := metadataFile.OperationsByDocURL(docsLink)
			if op == nil {
				noMatchCount++
				fmt.Println("no match", filepath.Base(method.Filename), method.ServiceName, method.MethodName, docsLink)
				continue
			}
			//op.GoMethod = fmt.Sprintf("%s.%s", method.ReceiverName, method.MethodName)

			gm := fmt.Sprintf("%s.%s", method.ServiceName, method.MethodName)
			if !slices.Contains(op.GoMethods, gm) {
				op.GoMethods = append(op.GoMethods, gm)
				sort.Strings(op.GoMethods)
			}
		}
	}
	fmt.Println(noMatchCount)
	//for _, op := range metadataFile.Operations {
	//	op.GoMethod = ""
	//	op.GoMethods = nil
	//}
	err = metadataFile.SaveFile(metadataFilename)
	require.NoError(t, err)
}

type ServiceMethod struct {
	ServiceName string
	MethodName  string
	Filename    string
	CodeComment string
}

var githubAPIDocsRE = regexp.MustCompile(`(?i)GitHub\s+API\s+docs:\s+(https://\S+)`)

// getDocsLinks is like getDocsLink but returns all links.
func getDocsLinks(comments string) []string {
	var links []string
	for _, line := range strings.Split(comments, "\n") {
		matches := githubAPIDocsRE.FindStringSubmatch(line)
		if len(matches) != 2 {
			continue
		}
		links = append(links, matches[1])
	}
	return links
}
