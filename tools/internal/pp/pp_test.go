package pp

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)


// realAstFileIterator implements astFileIterator.
type realAstFileIterator struct {
	fset   *token.FileSet
	pkgs   map[string]*ast.Package
	ch     chan *filenameAstFilePair
	closed bool
}

func (rafi *realAstFileIterator) Position(pos token.Pos) token.Position {
	return rafi.fset.Position(pos)
}

func (rafi *realAstFileIterator) Reset() {
	if !rafi.closed && rafi.ch != nil {
		logf("Closing old channel on Reset")
		close(rafi.ch)
	}
	rafi.ch = make(chan *filenameAstFilePair, 10)
	rafi.closed = false

	go func() {
		var count int
		for _, pkg := range rafi.pkgs {
			for filename, f := range pkg.Files {
				// logf("Sending file #%v: %v to channel", count, filename)
				rafi.ch <- &filenameAstFilePair{filename: filename, astFile: f}
				count++
			}
		}
		rafi.closed = true
		close(rafi.ch)
		logf("Closed channel after sending %v files", count)
		if count == 0 {
			log.Fatalf("Processed no files. Did you run this from the go-github directory?")
		}
	}()
}

func (rafi *realAstFileIterator) Next() *filenameAstFilePair {
	for pair := range rafi.ch {
		// logf("Next: returning file %v", pair.filename)
		return pair
	}
	return nil
}


func TestPP(t *testing.T) {
	fset := token.NewFileSet()
	sourceFilter := func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go") && !strings.HasPrefix(fi.Name(), "gen-")
	}
	err := os.Chdir("../../../github")
	require.NoError(t, err)
	pkgs, err := parser.ParseDir(fset, ".", sourceFilter, parser.ParseComments)
	require.NoError(t, err)
	services := findAllServices(pkgs)
	iter := &realAstFileIterator{fset: fset, pkgs: pkgs}
	endpoints, err := findAllServiceEndpoints(iter, services)
	require.NoError(t, err)
	for s := range endpoints {
		ep := endpoints[s]
		if len(ep.urlFormats) == 0 {
			fmt.Println("No urlFormats for", s)
		}
	}
}
