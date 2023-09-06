package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
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
	err := run(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	goghDir, err := internal.ProjRootDir(opts.workDir)
	if err != nil {
		return err
	}
	if opts.metadataFile == "" {
		opts.metadataFile = filepath.Join(goghDir, "metadata.yaml")
	}
	if opts.githubDir == "" {
		opts.githubDir = filepath.Join(goghDir, "github")
	}

	var metadata internal.Metadata
	err = internal.LoadMetadataFile(opts.metadataFile, &metadata)
	if err != nil {
		return err
	}
	dir, err := os.ReadDir(opts.githubDir)
	if err != nil {
		return err
	}
	for _, fi := range dir {
		if !strings.HasSuffix(fi.Name(), ".go") ||
			strings.HasPrefix(fi.Name(), "gen-") ||
			strings.HasSuffix(fi.Name(), "_test.go") {
			continue
		}
		err = updateFile(filepath.Join(opts.githubDir, fi.Name()), &metadata)
		if err != nil {
			return err
		}
	}
	return nil
}

var (
	docLineRE   = regexp.MustCompile(`(?i)\s*(//\s*)?GitHub\s+API\s+docs:`)
	emptyLineRE = regexp.MustCompile(`^\s*(//\s*)$`)
)

func updateFile(filename string, m *internal.Metadata) (errOut error) {
	fset := token.NewFileSet()
	df, err := decorator.ParseFile(fset, filename, nil, parser.ParseComments)
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

		// Get the method's receiver. It can be either an identifier or a pointer to an identifier.
		// This assumes all receivers are named and we don't have something like: `func (Client) Foo()`.
		methodName := d.Name.Name
		receiverType := ""
		switch x := d.Recv.List[0].Type.(type) {
		case *dst.Ident:
			receiverType = x.Name
		case *dst.StarExpr:
			receiverType = x.X.(*dst.Ident).Name
		}

		// create copy of comments without doc links
		var starts []string
		for _, s := range d.Decs.Start.All() {
			if !docLineRE.MatchString(s) {
				starts = append(starts, s)
			}
		}

		// remove trailing empty lines
		for len(starts) > 0 {
			if !emptyLineRE.MatchString(starts[len(starts)-1]) {
				break
			}
			starts = starts[:len(starts)-1]
		}

		docLinks := m.DocLinksForMethod(strings.Join([]string{receiverType, methodName}, "."))

		// add an empty line before adding doc links
		if len(docLinks) > 0 {
			starts = append(starts, "//")
		}

		for _, dl := range docLinks {
			starts = append(starts, fmt.Sprintf("// GitHub API docs: %s", dl))
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
