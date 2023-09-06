package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/google/go-github/tools/internal"
	"github.com/google/go-github/v54/github"
	"github.com/gregjones/httpcache"
	"github.com/gregjones/httpcache/diskcache"
	"golang.org/x/oauth2"
)

type options struct {
	workDir  string
	cacheDir string
	token    string
	ref      string
	filename string
}

func main() {
	ctx := context.Background()
	var opts options
	flag.StringVar(&opts.ref, "ref", "main", `git ref`)
	flag.StringVar(&opts.filename, "filename", "", `filename (default: "<go-github-root>/operations.yaml")`)
	flag.StringVar(&opts.workDir, "C", ".", `work directory -- must be in a go-github root`)
	flag.StringVar(&opts.cacheDir, "cache-dir", "", `cache directory (default: "<go-github-root>/tmp/update-metadata/cache")`)
	flag.Parse()
	goghDir, err := internal.ProjRootDir(opts.workDir)
	if err != nil {
		panic(err)
	}
	if opts.cacheDir == "" {
		opts.cacheDir = filepath.Join(goghDir, "tmp", "update-metadata", "cache")
	}
	if opts.filename == "" {
		opts.filename = filepath.Join(goghDir, "metadata.yaml")
	}
	opts.token = os.Getenv("GITHUB_TOKEN")
	err = run(ctx, opts)
	if err != nil {
		panic(err)
	}
}

func run(ctx context.Context, opts options) error {
	workDir := opts.workDir
	goghDir, err := internal.ProjRootDir(workDir)
	if err != nil {
		return err
	}
	cacheDir := opts.cacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(goghDir, "tmp", "update-metadata", "cache")
	}
	transport := http.DefaultTransport
	if opts.token != "" {
		transport = &oauth2.Transport{
			Base: transport,
			Source: oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: opts.token},
			),
		}
	}
	if opts.cacheDir != "" {
		transport = &httpcache.Transport{
			Transport:           transport,
			Cache:               diskcache.New(opts.cacheDir),
			MarkCachedResponses: true,
		}
	}
	client := github.NewClient(&http.Client{
		Transport: transport,
	})
	descs, err := internal.GetDescriptions(ctx, client, opts.ref)
	if err != nil {
		return err
	}
	opFile := &internal.Metadata{}
	err = internal.LoadMetadataFile(opts.filename, opFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}
	for _, op := range opFile.Operations {
		op.OpenAPIFiles = op.OpenAPIFiles[:0]
	}
	for _, desc := range descs {
		for p, pathItem := range desc.Description.Paths {
			for method, op := range pathItem.Operations() {
				docURL := ""
				if op.ExternalDocs != nil {
					docURL = op.ExternalDocs.URL
				}
				opFile.AddOperation(desc.Filename, &internal.OperationDesc{
					Method:           method,
					EndpointURL:      p,
					DocumentationURL: docURL,
					Summary:          op.Summary,
				})
			}
		}
	}
	sort.Slice(opFile.Operations, func(i, j int) bool {
		return opFile.Operations[i].Less(opFile.Operations[j])
	})
	return opFile.SaveFile(opts.filename)
}
