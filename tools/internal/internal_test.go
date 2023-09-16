package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"
)

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
	require.Equal(t, 1, len(methods))
	method := methods[0]
	require.Equal(t, "GitService.GetBlob", method.name())
	require.Equal(t, "GET", method.httpMethod)
}
