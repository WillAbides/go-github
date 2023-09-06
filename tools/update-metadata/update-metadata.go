package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/tools/internal"
	"github.com/google/go-github/v55/github"
	"github.com/gregjones/httpcache"
	"github.com/gregjones/httpcache/diskcache"
)

const description = `
update-metadata updates the metadata.yaml file from the OpenAPI descriptions in https://github.com/github/rest-api-description.
GITHUB_TOKEN must be set to a GitHub personal access token with the "public_repo" scope, and the working directory must within a go-github root.
`

func main() {
	ctx := context.Background()
	var workDir, cacheDir, ref, filename string
	flagset := flag.NewFlagSet("update-metadata", flag.ExitOnError)
	flagset.Usage = func() {
		fmt.Fprintf(
			flagset.Output(),
			"Usage: %s [options]\n\n%s\n\nOptions:\n",
			flagset.Name(),
			strings.TrimSpace(description),
		)
		flagset.PrintDefaults()
	}

	flagset.StringVar(&ref, "ref", "main", `git ref`)
	flagset.StringVar(&filename, "filename", "", `filename (default: "<go-github-root>/operations.yaml")`)
	flagset.StringVar(&workDir, "C", ".", `working directory`)
	flagset.StringVar(&cacheDir, "cache-dir", "", `cache directory (default: "<go-github-root>/tmp/update-metadata/cache")`)
	err := flagset.Parse(os.Args[1:])
	internal.ExitErr(err)

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		internal.ExitErr(fmt.Errorf("GITHUB_TOKEN environment variable must be set"))
	}
	goghDir, err := internal.ProjRootDir(workDir)
	internal.ExitErr(err)
	if cacheDir == "" {
		cacheDir = filepath.Join(goghDir, "tmp", "update-metadata", "cache")
	}
	if filename == "" {
		filename = filepath.Join(goghDir, "metadata.yaml")
	}

	client := github.NewClient(&http.Client{
		Transport: &httpcache.Transport{
			Transport: github.NewClient(nil).WithAuthToken(token).Client().Transport,
			Cache:     diskcache.New(cacheDir),
		},
	})

	opFile := &internal.Metadata{}
	err = internal.LoadMetadataFile(filename, opFile)
	internal.ExitErr(err)
	err = opFile.UpdateFromGithub(ctx, client.Repositories, ref)
	internal.ExitErr(err)
	err = opFile.SaveFile(filename)
	internal.ExitErr(err)
}
