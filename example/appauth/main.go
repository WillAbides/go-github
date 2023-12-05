// Copyright 2023 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// appauth demonstrates using the authenticating as a GitHub App and GitHub
// App Installation. To use this example, you must have a GitHub App and access
// to its private key file.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v57/github"
)

const defaultAPIURL = "https://api.github.com"

func main() {
	ctx := context.Background()

	// GITHUB_TOKEN is a personal access token used to get the GitHub App's ID from its slug.
	tkn := os.Getenv("GITHUB_TOKEN")
	if tkn == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	var keyFile, apiURL, appSlug string
	flag.StringVar(&keyFile, "key", "", "path to private key file")
	flag.StringVar(&apiURL, "api-url", defaultAPIURL, "GitHub API URL")
	flag.StringVar(&appSlug, "app", "", "GitHub App slug")
	flag.Parse()

	if appSlug == "" {
		log.Fatal("-app is required")
	}
	if keyFile == "" {
		log.Fatal("-key is required")
	}

	err := listAppInstRepos(ctx, apiURL, tkn, appSlug, keyFile)
	if err != nil {
		log.Fatal(err)
	}
}

func listAppInstRepos(ctx context.Context, apiURL, pat, appSlug string, keyfile string) error {
	// Create the base client without authentication.
	client, err := github.NewClient(nil).WithEnterpriseURLs(apiURL, "")
	if err != nil {
		return err
	}

	// Authenticate with the personal access token to get the app from its slug.
	client = client.WithAuthToken(pat)

	app, _, err := client.Apps.Get(ctx, appSlug)
	if err != nil {
		return err
	}

	fmt.Printf("App %s %d:\n", app.GetName(), app.GetID())

	// Use ghinstallation to create the auth provider for the app.
	appAuth, err := ghinstallation.NewAppsTransportKeyFromFile(http.DefaultTransport, app.GetID(), keyfile)
	if err != nil {
		return err
	}

	var pageOpt github.ListOptions
	for {
		// Authenticate as the app to list installations.
		client = client.WithTokenSource(appAuth)

		installations, resp, err := client.Apps.ListInstallations(ctx, &pageOpt)
		if err != nil {
			return err
		}

		for _, inst := range installations {
			fmt.Printf("  Installation %d:\n", inst.GetID())
			fmt.Printf("    Repositories:\n")

			// Use ghinstallation to create the auth provider for this installation.
			instAuth := ghinstallation.NewFromAppsTransport(appAuth, inst.GetID())

			// Authenticate as the installation to list repositories.
			client = client.WithTokenSource(instAuth)

			var instPageOpt github.ListOptions
			for {
				repos, resp, err := client.Apps.ListRepos(ctx, &instPageOpt)
				if err != nil {
					return err
				}
				for _, repo := range repos.Repositories {
					fmt.Printf("      %s\n", repo.GetFullName())
				}
				if resp.NextPage == 0 {
					break
				}
				instPageOpt.Page = resp.NextPage
			}
		}

		if resp.NextPage == 0 {
			break
		}
		pageOpt.Page = resp.NextPage
	}

	return nil
}
