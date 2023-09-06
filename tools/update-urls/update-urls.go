package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/tools/internal"
)

const description = `
update-urls updates the documentation URLs in the Go source files in the github directory to match the urls in the metadata file.
The working directory must be within a go-github root.
`

func main() {
	var workDir, metadataFile, githubDir string

	flag.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			"Usage: update-urls [options]\n\n%s\n\nOptions:\n",
			strings.TrimSpace(description),
		)
		flag.PrintDefaults()
	}

	flag.StringVar(&workDir, "C", ".", `working directory`)
	flag.StringVar(&metadataFile, "metadata-file", "", `metadata file (default: "<go-github-root>/metadata.yaml")`)
	flag.StringVar(&githubDir, "github-dir", "", `github directory (default: "<go-github-root>/github")`)
	flag.Parse()

	goghDir, err := internal.ProjRootDir(workDir)
	internal.ExitErr(err)
	if metadataFile == "" {
		metadataFile = filepath.Join(goghDir, "metadata.yaml")
	}
	if githubDir == "" {
		githubDir = filepath.Join(goghDir, "github")
	}

	var metadata internal.Metadata
	err = internal.LoadMetadataFile(metadataFile, &metadata)
	internal.ExitErr(err)
	dir, err := os.ReadDir(githubDir)
	internal.ExitErr(err)
	var content, updatedContent []byte
	for _, fi := range dir {
		if !strings.HasSuffix(fi.Name(), ".go") ||
			strings.HasSuffix(fi.Name(), "_test.go") {
			continue
		}
		filename := filepath.Join(githubDir, fi.Name())
		content, err = os.ReadFile(filename)
		internal.ExitErr(err)
		updatedContent, err = internal.UpdateDocsLinks(&metadata, content)
		err = os.WriteFile(filename, updatedContent, 0)
		internal.ExitErr(err)
	}
}
