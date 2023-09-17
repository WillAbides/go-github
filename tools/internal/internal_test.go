package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"
)

func TestPopulateOps(t *testing.T) {
	var meta Metadata
	err := LoadMetadataFile("../../metadata.yaml", &meta)
	require.NoError(t, err)
	var zeroDesc OperationDesc
	meta.ManualOps = meta.ManualOps[:0]
	meta.OpenapiOps = meta.OpenapiOps[:0]
	meta.OverrideOps = meta.OverrideOps[:0]
	for _, op := range meta.OldOps {
		if op.OpenAPI == zeroDesc {
			meta.ManualOps = append(meta.ManualOps, &Operation2{
				Name:             op.ID,
				DocumentationURL: op.DocumentationURL(),
			})
			continue
		}
		meta.OpenapiOps = append(meta.OpenapiOps, &Operation2{
			Name:             op.ID,
			DocumentationURL: op.OpenAPI.DocumentationURL,
			OpenAPIFiles:     op.OpenAPIFiles,
		})
		if op.Override == zeroDesc {
			continue
		}
		meta.OverrideOps = append(meta.OverrideOps, &Operation2{
			Name:             op.ID,
			DocumentationURL: op.Override.DocumentationURL,
		})
	}
	err = meta.SaveFile("../../metadata.yaml")
	require.NoError(t, err)
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
