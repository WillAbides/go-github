package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/google/go-github/tools/internal"
)

type options struct {
	workDir      string
	metadataFile string
	githubDir    string
}

func main() {
	var opts options
	flag.StringVar(&opts.workDir, "C", ".", `work directory -- must be in a go-github root`)
	flag.StringVar(&opts.metadataFile, "metadata-file", "", `metadata file (default: "<go-github-root>/metadata.yaml")`)
	flag.StringVar(&opts.githubDir, "github-dir", "", `github directory (default: "<go-github-root>/github")`)
	flag.Parse()
	goghDir, err := internal.ProjRootDir(opts.workDir)
	if err != nil {
		panic(err)
	}
	if opts.metadataFile == "" {
		opts.metadataFile = filepath.Join(goghDir, "metadata.yaml")
	}
	if opts.githubDir == "" {
		opts.githubDir = filepath.Join(goghDir, "github")
	}
	err = run(opts)
	if err != nil {
		panic(err)
	}
}

func run(opts options) error {
	metadataFile := &internal.Metadata{}
	err := internal.LoadMetadataFile(opts.metadataFile, metadataFile)
	if err != nil {
		return err
	}
	noTestFilesFilter := func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, opts.githubDir, noTestFilesFilter, parser.ParseComments)
	if err != nil {
		return err
	}
	ghPkg := pkgs["github"]
	if ghPkg == nil {
		return fmt.Errorf("no github package found in %s", opts.githubDir)
	}
	for k := range ghPkg.Files {
		err = updateFile(fset, ghPkg.Files[k], metadataFile)
		if err != nil {
			return err
		}
	}
	return nil
}

var (
	docLineRE = regexp.MustCompile(`(?i)\s*(//\s*)?GitHub\s+API\s+docs:`)
	emptyLineRE = regexp.MustCompile(`^\s*(//\s*)$`)
)

func updateFile(fset *token.FileSet, af *ast.File, m *internal.Metadata) (errOut error) {
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

		// remove empty lines from end of starts
		for len(starts) > 0 && emptyLineRE.MatchString(starts[len(starts)-1]) {
			starts = starts[:len(starts)-1]
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
