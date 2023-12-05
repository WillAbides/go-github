// Copyright 2021 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// newfilewithappauth demonstrates the functionality of GitHub's app authentication
// methods by fetching an installation access token and reauthenticating to GitHub
// with OAuth configurations.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v57/github"
)

func main() {
	const gitHost = "https://git.api.com"

	privatePem, err := os.ReadFile("path/to/pem")
	if err != nil {
		log.Fatalf("failed to read pem: %v", err)
	}

	// create authentication provider for the app
	appAuth, err := ghinstallation.NewAppsTransport(http.DefaultTransport, 10, privatePem)
	if err != nil {
		log.Fatalf("faild to create app transport: %v\n", err)
	}
	appAuth.BaseURL = gitHost

	// create git client with the app auth
	client, err := github.NewClient(nil).
		WithTokenSource(appAuth).
		WithEnterpriseURLs(gitHost, gitHost)
	if err != nil {
		log.Fatalf("faild to create git client for app: %v\n", err)
	}

	installations, _, err := client.Apps.ListInstallations(context.Background(), &github.ListOptions{})
	if err != nil {
		log.Fatalf("failed to list installations: %v\n", err)
	}

	//capture our installationId for our app
	//we need this for the access token
	var installID int64
	for _, val := range installations {
		installID = val.GetID()
	}

	// create an authentication provider for the installation
	instAuth := ghinstallation.NewFromAppsTransport(appAuth, installID)

	// update the client to use the installation auth
	client = client.WithTokenSource(instAuth)

	_, resp, err := client.Repositories.CreateFile(
		context.Background(),
		"repoOwner",
		"sample-repo",
		"example/foo.txt",
		&github.RepositoryContentFileOptions{
			Content: []byte("foo"),
			Message: github.String("sample commit"),
			SHA:     nil,
		})
	if err != nil {
		log.Fatalf("failed to create new file: %v\n", err)
	}

	log.Printf("file written status code: %v", resp.StatusCode)
}
