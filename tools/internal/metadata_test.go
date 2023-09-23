package internal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func copyDir(dst, src string) error {
	dst, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	return filepath.Walk(src, func(srcPath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, strings.TrimPrefix(srcPath, src))
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		srcContent, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, srcContent, info.Mode())
	})
}

func assertEqualDirContents(t *testing.T, want, got string) bool {
	t.Helper()
	pass := true
	err := filepath.Walk(want, func(wantPath string, info fs.FileInfo, err error) error {
		t.Helper()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		wantContent, err := os.ReadFile(wantPath)
		if err != nil {
			return err
		}
		gotPath := filepath.Join(got, strings.TrimPrefix(wantPath, want))
		gotContent, err := os.ReadFile(gotPath)
		if err != nil {
			return err
		}
		ok := assert.Equal(t, string(wantContent), string(gotContent))
		if !ok {
			pass = false
		}
		return nil
	})
	if err != nil {
		t.Error(err)
		pass = false
	}
	return pass
}

func checkGoldenDir(t *testing.T, golden, got string) bool {
	t.Helper()
	if os.Getenv("UPDATE_GOLDEN") == "" {
		return assertEqualDirContents(t, golden, got)
	}
	err := os.RemoveAll(golden)
	if !assert.NoError(t, err) {
		return false
	}
	err = copyDir(golden, got)
	return assert.NoError(t, err)
}

func TestFoo(t *testing.T) {
	srcDir := filepath.FromSlash("testdata/updatedocs")
	tmpDir := t.TempDir()
	err := copyDir(tmpDir, srcDir)
	require.NoError(t, err)
	meta, err := LoadMetadataFile(filepath.Join(tmpDir, "metadata.yaml"))
	require.NoError(t, err)
	err = UpdateDocLinks(meta, tmpDir)
	require.NoError(t, err)
	checkGoldenDir(t, srcDir + "-golden", tmpDir)
}

func Test_updateDocsLinksInFile(t *testing.T) {
	content := `
package github

// RemoveRunnerGroupRunners removes a self-hosted runner from a group configured in an organization.
// The runner is then returned to the default group.
//
// GitHub API docs: https://docs.github.com/enterprise-cloud@latest/rest/actions/self-hosted-runner-groups#remove-a-self-hosted-runner-from-a-group-for-an-organization
// GitHub API docs: https://docs.github.com/enterprise-cloud@latest/rest/actions/something/else
func (s *ActionsService) RemoveRunnerGroupRunners(ctx context.Context, org string, groupID, runnerID int64) (*Response, error) {
	panic("not implemented")
}
`
	meta := &Metadata{
		Methods: []*Method{
			{
				Name: "ActionsService.RemoveRunnerGroupRunners",
				OpNames: []string{
					"DELETE /orgs/{org}/actions/runner-groups/{runner_group_id}/runners/{runner_id}",
				},
			},
		},
		OpenapiOps: []*Operation{
			{
				Name:             "DELETE /orgs/{org}/actions/runner-groups/{runner_group_id}/runners/{runner_id}",
				DocumentationURL: "https://docs.github.com/enterprise-cloud@latest//rest/actions/self-hosted-runner-groups#remove-a-self-hosted-runner-from-a-group-for-an-organization",
			},
		},
	}

	got, err := updateDocsLinksInFile(meta, []byte(content))
	require.NoError(t, err)
	fmt.Println(string(got))
}
