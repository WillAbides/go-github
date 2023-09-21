package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"
)

func TestCanonical(t *testing.T) {
	meta, err := LoadMetadataFile("../../metadata.yaml")
	require.NoError(t, err)
	err = meta.CanonizeMethodOperations()
	require.NoError(t, err)
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
	create := methodsMap["ActionsService.CreateWorkflowDispatchEventByFileName"]
	require.NotNil(t, create)
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
	fmt.Println(method.name())
}
