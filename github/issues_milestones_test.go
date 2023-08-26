// Copyright 2014 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestIssuesService_ListMilestones(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/milestones", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"state":     "closed",
			"sort":      "due_date",
			"direction": "asc",
			"page":      "2",
		})
		fmt.Fprint(w, `[{"number":1}]`)
	})

	opt := &MilestoneListOptions{"closed", "due_date", "asc", ListOptions{Page: 2}}
	ctx := context.Background()
	milestones, _, err := client.Issues.ListMilestones(ctx, "o", "r", opt)
	if err != nil {
		t.Errorf("IssuesService.ListMilestones returned error: %v", err)
	}

	want := []*Milestone{{Number: Int(1)}}
	if !cmp.Equal(milestones, want) {
		t.Errorf("IssuesService.ListMilestones returned %+v, want %+v", milestones, want)
	}

	const methodName = "ListMilestones"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Issues.ListMilestones(ctx, "\n", "\n", opt)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Issues.ListMilestones(ctx, "o", "r", opt)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestIssuesService_ListMilestones_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Issues.ListMilestones(ctx, "%", "r", nil)
	testURLParseError(t, err)
}

func TestIssuesService_GetMilestone(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/milestones/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"number":1}`)
	})

	ctx := context.Background()
	milestone, _, err := client.Issues.GetMilestone(ctx, "o", "r", 1)
	if err != nil {
		t.Errorf("IssuesService.GetMilestone returned error: %v", err)
	}

	want := &Milestone{Number: Int(1)}
	if !cmp.Equal(milestone, want) {
		t.Errorf("IssuesService.GetMilestone returned %+v, want %+v", milestone, want)
	}

	const methodName = "GetMilestone"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Issues.GetMilestone(ctx, "\n", "\n", -1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Issues.GetMilestone(ctx, "o", "r", 1)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestIssuesService_GetMilestone_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Issues.GetMilestone(ctx, "%", "r", 1)
	testURLParseError(t, err)
}

func TestIssuesService_CreateMilestone(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &Milestone{Title: String("t")}

	mux.HandleFunc("/repos/o/r/milestones", func(w http.ResponseWriter, r *http.Request) {
		v := new(Milestone)
		assertNilError(t, json.NewDecoder(r.Body).Decode(v))

		testMethod(t, r, "POST")
		if !cmp.Equal(v, input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}

		fmt.Fprint(w, `{"number":1}`)
	})

	ctx := context.Background()
	milestone, _, err := client.Issues.CreateMilestone(ctx, "o", "r", input)
	if err != nil {
		t.Errorf("IssuesService.CreateMilestone returned error: %v", err)
	}

	want := &Milestone{Number: Int(1)}
	if !cmp.Equal(milestone, want) {
		t.Errorf("IssuesService.CreateMilestone returned %+v, want %+v", milestone, want)
	}

	const methodName = "CreateMilestone"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Issues.CreateMilestone(ctx, "\n", "\n", input)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Issues.CreateMilestone(ctx, "o", "r", input)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestIssuesService_CreateMilestone_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Issues.CreateMilestone(ctx, "%", "r", nil)
	testURLParseError(t, err)
}

func TestIssuesService_EditMilestone(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &Milestone{Title: String("t")}

	mux.HandleFunc("/repos/o/r/milestones/1", func(w http.ResponseWriter, r *http.Request) {
		v := new(Milestone)
		assertNilError(t, json.NewDecoder(r.Body).Decode(v))

		testMethod(t, r, "PATCH")
		if !cmp.Equal(v, input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}

		fmt.Fprint(w, `{"number":1}`)
	})

	ctx := context.Background()
	milestone, _, err := client.Issues.EditMilestone(ctx, "o", "r", 1, input)
	if err != nil {
		t.Errorf("IssuesService.EditMilestone returned error: %v", err)
	}

	want := &Milestone{Number: Int(1)}
	if !cmp.Equal(milestone, want) {
		t.Errorf("IssuesService.EditMilestone returned %+v, want %+v", milestone, want)
	}

	const methodName = "EditMilestone"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Issues.EditMilestone(ctx, "\n", "\n", -1, input)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Issues.EditMilestone(ctx, "o", "r", 1, input)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestIssuesService_EditMilestone_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Issues.EditMilestone(ctx, "%", "r", 1, nil)
	testURLParseError(t, err)
}

func TestIssuesService_DeleteMilestone(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/milestones/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
	})

	ctx := context.Background()
	_, err := client.Issues.DeleteMilestone(ctx, "o", "r", 1)
	if err != nil {
		t.Errorf("IssuesService.DeleteMilestone returned error: %v", err)
	}

	const methodName = "DeleteMilestone"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Issues.DeleteMilestone(ctx, "\n", "\n", -1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Issues.DeleteMilestone(ctx, "o", "r", 1)
	})
}

func TestIssuesService_DeleteMilestone_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, err := client.Issues.DeleteMilestone(ctx, "%", "r", 1)
	testURLParseError(t, err)
}

func TestMilestone_Marshal(t *testing.T) {
	testJSONMarshal(t, &Milestone{}, "{}")

	u := &Milestone{
		URL:         String("url"),
		HTMLURL:     String("hurl"),
		LabelsURL:   String("lurl"),
		ID:          Int64(1),
		Number:      Int(1),
		State:       String("state"),
		Title:       String("title"),
		Description: String("desc"),
		Creator: &User{
			Login:           String("l"),
			ID:              Int64(1),
			URL:             String("u"),
			AvatarURL:       String("a"),
			GravatarID:      String("g"),
			Name:            String("n"),
			Company:         String("c"),
			Blog:            String("b"),
			Location:        String("l"),
			Email:           String("e"),
			Hireable:        Bool(true),
			Bio:             String("b"),
			TwitterUsername: String("tu"),
			PublicRepos:     Int(1),
			Followers:       Int(1),
			Following:       Int(1),
			CreatedAt:       &Timestamp{referenceTime},
			SuspendedAt:     &Timestamp{referenceTime},
		},
		OpenIssues:   Int(1),
		ClosedIssues: Int(1),
		CreatedAt:    &Timestamp{referenceTime},
		UpdatedAt:    &Timestamp{referenceTime},
		ClosedAt:     &Timestamp{referenceTime},
		DueOn:        &Timestamp{referenceTime},
		NodeID:       String("nid"),
	}

	want := `{
		"url": "url",
		"html_url": "hurl",
		"labels_url": "lurl",
		"id": 1,
		"number": 1,
		"state": "state",
		"title": "title",
		"description": "desc",
		"creator": {
			"login": "l",
			"id": 1,
			"avatar_url": "a",
			"gravatar_id": "g",
			"name": "n",
			"company": "c",
			"blog": "b",
			"location": "l",
			"email": "e",
			"hireable": true,
			"bio": "b",
			"twitter_username": "tu",
			"public_repos": 1,
			"followers": 1,
			"following": 1,
			"created_at": ` + referenceTimeStr + `,
			"suspended_at": ` + referenceTimeStr + `,
			"url": "u"
		},
		"open_issues": 1,
		"closed_issues": 1,
		"created_at": ` + referenceTimeStr + `,
		"updated_at": ` + referenceTimeStr + `,
		"closed_at": ` + referenceTimeStr + `,
		"due_on": ` + referenceTimeStr + `,
		"node_id": "nid"
	}`

	testJSONMarshal(t, u, want)
}
