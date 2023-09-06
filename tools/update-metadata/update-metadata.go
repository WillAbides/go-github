package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/tools/internal"
	"github.com/google/go-github/v55/github"
)

const description = `
update-metadata updates the metadata.yaml file from the OpenAPI descriptions in https://github.com/github/rest-api-description.
GITHUB_TOKEN must be set to a GitHub personal access token with the "public_repo" scope, and the working directory must within a go-github root.
`

func main() {
	ctx := context.Background()
	var workDir, ref, filename string

	flag.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			"Usage: update-metadata [options]\n\n%s\n\nOptions:\n",
			strings.TrimSpace(description),
		)
		flag.PrintDefaults()
	}

	flag.StringVar(&ref, "ref", "main", `git ref`)
	flag.StringVar(&filename, "filename", "", `filename (default: "<go-github-root>/metadata.yaml")`)
	flag.StringVar(&workDir, "C", ".", `working directory`)
	flag.Parse()

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		internal.ExitErr(fmt.Errorf("GITHUB_TOKEN environment variable must be set"))
	}
	goghDir, err := internal.ProjRootDir(workDir)
	internal.ExitErr(err)
	if filename == "" {
		filename = filepath.Join(goghDir, "metadata.yaml")
	}

	opFile := &internal.Metadata{}
	err = internal.LoadMetadataFile(filename, opFile)
	internal.ExitErr(err)
	err = opFile.UpdateFromGithub(ctx, github.NewClient(nil).WithAuthToken(token).Repositories, ref)
	internal.ExitErr(err)
	err = opFile.SaveFile(filename)
	internal.ExitErr(err)
}
