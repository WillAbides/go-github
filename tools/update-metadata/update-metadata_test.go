package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-github/tools/internal"
	"github.com/stretchr/testify/require"
)

func TestUncovered(t *testing.T) {
	gghRoot, err := internal.ProjRootDir(".")
	metadataFilename := filepath.Join(gghRoot, "metadata.yaml")
	metadataFile := &internal.Metadata{}
	err = internal.LoadMetadataFile(metadataFilename, metadataFile)
	require.NoError(t, err)
	count := 0
	for _, op := range metadataFile.Operations {
		if ! slices.Contains(op.OpenAPIFiles, "descriptions/api.github.com/api.github.com.json") {
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
			//op.GoMethod = fmt.Sprintf("%s.%s", method.ServiceName, method.MethodName)

			gm := fmt.Sprintf("%s.%s", method.ServiceName, method.MethodName)
			if ! slices.Contains(op.GoMethods, gm) {
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
