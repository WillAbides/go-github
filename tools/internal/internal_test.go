package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"
)

func TestDups(t *testing.T) {
	var meta Metadata
	err := LoadMetadataFile("../../metadata.yaml", &meta)
	require.NoError(t, err)
	normOps := map[string][]*Operation{}
	for _, op := range meta.Operations {
		verb := op.Method()
		u := normalizedURL(op.EndpointURL())
		opID := verb + " " + u
		normOps[opID] = append(normOps[opID], op)
	}
	for opID, ops := range normOps {
		if len(ops) > 1 {
			fmt.Println(opID)
			for _, op := range ops {
				fmt.Println("  ", op.DocumentationURL())
			}
		}
	}
	fmt.Println(len(normOps))
}

func extractTxtar(t *testing.T, filename string) string {
	t.Helper()
	a, err := txtar.ParseFile(filepath.FromSlash(filename))
	require.NoError(t, err)
	dir := t.TempDir()
	for _, f := range a.Files {
		name := filepath.Join(dir, f.Name)
		err = os.WriteFile(name, f.Data, 0600)
		require.NoError(t, err)
	}
	return dir
}

func TestFoo(t *testing.T) {
	dir := extractTxtar(t, "testdata/test1.txtar")
	methods, err := getServiceMethods(dir)
	require.NoError(t, err)
	require.Equal(t, 2, len(methods))
	methodsMap := map[string]*serviceMethod{}
	for _, m := range methods {
		methodsMap[m.name()] = m
	}
	getBlob := methodsMap["GitService.GetBlob"]
	require.NotNil(t, getBlob)
	require.Equal(t, "GET", getBlob.httpMethod)
	//"CreateWorkflowDispatchEventByFileName"
	create := methodsMap["ActionsService.CreateWorkflowDispatchEventByFileName"]
	require.NotNil(t, create)
	require.Equal(t, "ActionsService.createWorkflowDispatchEvent", create.helper)
	//require.Equal(t, "POST", create.httpMethod)

}

func TestBar(t *testing.T) {
	dir := "../../github"
	methods, err := getServiceMethods(dir)
	require.NoError(t, err)
	methodsMap := map[string]*serviceMethod{}
	for _, m := range methods {
		methodsMap[m.name()] = m
	}
	method, ok := methodsMap["MarketplaceService.GetPlanAccountForAccount"]
	require.True(t, ok)
	fmt.Println(method.urls)

}
