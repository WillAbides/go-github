// Copyright 2023 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

type OperationDesc struct {
	DocumentationURL string `yaml:"documentation_url,omitempty" json:"documentation_url,omitempty"`
}

type Operation2 struct {
	Name             string   `yaml:"name,omitempty" json:"name,omitempty"`
	DocumentationURL string   `yaml:"documentation_url,omitempty" json:"documentation_url,omitempty"`
	OpenAPIFiles     []string `yaml:"openapi_files,omitempty" json:"openapi_files,omitempty"`
}

type Operation struct {
	ID           string        `yaml:"id,omitempty" json:"id,omitempty"`
	OpenAPI      OperationDesc `yaml:"openapi,omitempty" json:"openapi,omitempty"`
	Override     OperationDesc `yaml:"override,omitempty" json:"override,omitempty"`
	OpenAPIFiles []string      `yaml:"openapi_files,omitempty" json:"openapi_files,omitempty"`
}

type operationJSON struct {
	ID          string   `json:"id,omitempty"`
	DocumentURL string   `json:"documentation_url,omitempty"`
	Plans       []string `json:"plans,omitempty"`
}

func (o *Operation) MarshalJSON() ([]byte, error) {
	return json.Marshal(&operationJSON{
		ID:          o.ID,
		Plans:       o.Plans(),
		DocumentURL: o.DocumentationURL(),
	})
}

func (o *Operation) Plans() []string {
	var plans []string
	if slices.ContainsFunc(o.OpenAPIFiles, func(s string) bool {
		return strings.HasSuffix(s, "api.github.com.json")
	}) {
		plans = append(plans, "public")
	}
	if slices.ContainsFunc(o.OpenAPIFiles, func(s string) bool {
		return strings.HasSuffix(s, "ghec.json")
	}) {
		plans = append(plans, "ghec")
	}
	if slices.ContainsFunc(o.OpenAPIFiles, func(s string) bool {
		return strings.Contains(s, "/ghes")
	}) {
		plans = append(plans, "ghes")
	}
	return plans
}

func (o *Operation) DocumentationURL() string {
	if o.Override.DocumentationURL != "" {
		return o.Override.DocumentationURL
	}
	return o.OpenAPI.DocumentationURL
}

func (o *Operation) Less(other *Operation) bool {
	leftVerb, leftURL := parseID(o.ID)
	rightVerb, rightURL := parseID(other.ID)
	if leftURL != rightURL {
		return leftURL < rightURL
	}
	return leftVerb < rightVerb
}

func (o *Operation2) Less(other *Operation2) bool {
	leftVerb, leftURL := parseID(o.Name)
	rightVerb, rightURL := parseID(other.Name)
	if leftURL != rightURL {
		return leftURL < rightURL
	}
	return leftVerb < rightVerb
}

func sortOperations(ops []*Operation2) {
	sort.Slice(ops, func(i, j int) bool {
		return ops[i].Less(ops[j])
	})
}

func (o *Operation) normalizedID() string {
	verb, u := parseID(o.ID)
	return verb + " " + normalizedURL(u)
}

var normalizedURLs = map[string]string{}
var normalizedURLsMu sync.Mutex

// normalizedURL returns an endpoint with all templated path parameters replaced with *.
func normalizedURL(u string) string {
	normalizedURLsMu.Lock()
	defer normalizedURLsMu.Unlock()
	n, ok := normalizedURLs[u]
	if ok {
		return n
	}
	parts := strings.Split(u, "/")
	for i, p := range parts {
		if len(p) > 0 && p[0] == '{' {
			parts[i] = "*"
		}
	}
	n = strings.Join(parts, "/")
	normalizedURLs[u] = n
	return n
}

func normalizedID(id string) string {
	verb, u := parseID(id)
	return verb + " " + normalizedURL(u)
}

func parseID(id string) (verb, url string) {
	verb, url, _ = strings.Cut(id, " ")
	return verb, url
}

type Metadata struct {
	MethodOperations    map[string][]string `yaml:"method_operations,omitempty"`
	UndocumentedMethods []string            `yaml:"undocumented_methods,omitempty"`
	ManualOps           []*Operation2       `yaml:"operations"`
	OverrideOps         []*Operation2       `yaml:"operation_overrides"`
	OpenapiOps          []*Operation2       `yaml:"openapi_operations"`
	OldOps              []*Operation        `yaml:"old_operations,omitempty"`

	mu          sync.Mutex
	resolvedOps map[string]*Operation2
}

func (m *Metadata) resolve() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.resolvedOps != nil {
		return
	}
	m.resolvedOps = map[string]*Operation2{}
	for _, op := range m.OpenapiOps {
		m.resolvedOps[op.Name] = op
	}
	for _, op := range m.ManualOps {
		m.resolvedOps[op.Name] = op
	}
	for _, op := range m.OverrideOps {
		_, ok := m.resolvedOps[op.Name]
		if !ok {
			m.resolvedOps[op.Name] = op
		}
		if op.DocumentationURL != "" {
			m.resolvedOps[op.Name].DocumentationURL = op.DocumentationURL
		}
		if len(op.OpenAPIFiles) > 0 {
			m.resolvedOps[op.Name].OpenAPIFiles = op.OpenAPIFiles
		}
	}
}

func (m *Metadata) Operations() []*Operation2 {
	m.resolve()
	ops := make([]*Operation2, 0, len(m.resolvedOps))
	for _, op := range m.resolvedOps {
		ops = append(ops, op)
	}
	sortOperations(ops)
	return ops
}

func LoadMetadataFile(filename string, opFile *Metadata) (errOut error) {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if errOut == nil {
			errOut = e
		}
	}()
	return yaml.NewDecoder(f).Decode(opFile)
}

func (m *Metadata) SaveFile(filename string) (errOut error) {
	sort.Slice(m.OldOps, func(i, j int) bool {
		return m.OldOps[i].Less(m.OldOps[j])
	})
	sort.Slice(m.ManualOps, func(i, j int) bool {
		return m.ManualOps[i].Less(m.ManualOps[j])
	})
	sort.Slice(m.OverrideOps, func(i, j int) bool {
		return m.OverrideOps[i].Less(m.OverrideOps[j])
	})
	sort.Slice(m.OpenapiOps, func(i, j int) bool {
		return m.OpenapiOps[i].Less(m.OpenapiOps[j])
	})
	for i := range m.MethodOperations {
		sort.Strings(m.MethodOperations[i])
	}
	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer func() {
		e := f.Close()
		if errOut == nil {
			errOut = e
		}
	}()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	return enc.Encode(m)
}

func (m *Metadata) addOperation(filename string, descID, docURL string) {
	normDescID := normalizedID(descID)
	for _, op := range m.OldOps {
		if normDescID != normalizedID(op.ID) {
			continue
		}
		if len(op.OpenAPIFiles) == 0 {
			op.OpenAPIFiles = append(op.OpenAPIFiles, filename)
			op.OpenAPI = OperationDesc{
				DocumentationURL: docURL,
			}
			return
		}
		// just append to files, but only add the first ghes file
		if !strings.Contains(filename, "/ghes") {
			op.OpenAPIFiles = append(op.OpenAPIFiles, filename)
			return
		}
		for _, f := range op.OpenAPIFiles {
			if strings.Contains(f, "/ghes") {
				return
			}
		}
		op.OpenAPIFiles = append(op.OpenAPIFiles, filename)
		return
	}

	m.OldOps = append(m.OldOps, &Operation{
		ID:           descID,
		OpenAPIFiles: []string{filename},
		OpenAPI: OperationDesc{
			DocumentationURL: docURL,
		},
	})
}

// OperationMethods returns a list methods that are mapped to the given operation id.
func (m *Metadata) OperationMethods(opID string) []string {
	var methods []string
	for method, methodOpIDs := range m.MethodOperations {
		for _, methodOpID := range methodOpIDs {
			if methodOpID == opID {
				methods = append(methods, method)
			}
		}
	}
	return methods
}

func (m *Metadata) operationsByID(id string) []*Operation {
	var operations []*Operation
	for _, op := range m.OldOps {
		if op.ID == id {
			operations = append(operations, op)
		}
	}
	return operations
}

func (m *Metadata) operationsForMethod(method string) []*Operation {
	if m.MethodOperations == nil {
		return nil
	}
	var operations []*Operation
	for _, id := range m.MethodOperations[method] {
		operations = append(operations, m.operationsByID(id)...)
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Less(operations[j])
	})
	return operations
}

func (m *Metadata) UpdateFromGithub(ctx context.Context, client contentsClient, ref string) error {
	descs, err := getDescriptions(ctx, client, ref)
	if err != nil {
		return err
	}
	for _, op := range m.OldOps {
		op.OpenAPIFiles = op.OpenAPIFiles[:0]
	}
	for _, desc := range descs {
		for p, pathItem := range desc.description.Paths {
			for method, op := range pathItem.Operations() {
				docURL := ""
				if op.ExternalDocs != nil {
					docURL = op.ExternalDocs.URL
				}
				id := method + " " + p
				m.addOperation(desc.filename, id, docURL)
			}
		}
	}
	sort.Slice(m.OldOps, func(i, j int) bool {
		return m.OldOps[i].Less(m.OldOps[j])
	})
	return nil
}

// UpdateDocLinks updates the code comments in dir with doc urls from metadata.
func UpdateDocLinks(meta *Metadata, dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() ||
			!strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		updatedContent, err := updateDocsLinksInFile(meta, content)
		if err != nil {
			return err
		}
		if bytes.Equal(content, updatedContent) {
			return nil
		}
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		_, err = f.Write(updatedContent)
		if err != nil {
			return err
		}
		return f.Close()
	})
}

// updateDocsLinksInFile updates in the code comments in content with doc urls from metadata.
func updateDocsLinksInFile(metadata *Metadata, content []byte) ([]byte, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// ignore files where package is not github
	if node.Name.Name != "github" {
		return content, nil
	}

	ast.Inspect(node, func(n ast.Node) bool {
		return updateDocsLinksForNode(metadata, n)
	})

	var buf bytes.Buffer
	err = printer.Fprint(&buf, fset, node)
	if err != nil {
		return nil, err
	}
	return format.Source(buf.Bytes())
}

var (
	docLineRE   = regexp.MustCompile(`(?i)\s*(//\s*)?GitHub\s+API\s+docs:\s*(https?://\S+)`)
	emptyLineRE = regexp.MustCompile(`^\s*(//\s*)$`)
)

func updateDocsLinksForNode(metadata *Metadata, n ast.Node) bool {
	fn, ok := n.(*ast.FuncDecl)
	if !ok || !fn.Name.IsExported() {
		return true
	}
	methodName := fn.Name.Name

	// Get the method's receiver. It can be either an identifier or a pointer to an identifier.
	// This assumes all receivers are named and we don't have something like: `func (Client) Foo()`.
	receiverType := ""
	if fn.Recv != nil {
		switch x := fn.Recv.List[0].Type.(type) {
		case *ast.Ident:
			receiverType = x.Name
		case *ast.StarExpr:
			receiverType = x.X.(*ast.Ident).Name
		}
	}
	if !ast.IsExported(receiverType) {
		return true
	}

	linksMap := map[string]struct{}{}
	ops := metadata.operationsForMethod(strings.Join([]string{receiverType, methodName}, "."))
	for _, op := range ops {
		linksMap[op.DocumentationURL()] = struct{}{}
	}

	// create copy of comment group with non-matching doc links removed
	if fn.Doc == nil {
		fn.Doc = &ast.CommentGroup{}
	}
	fnComments := make([]*ast.Comment, 0, len(fn.Doc.List))
	for _, comment := range fn.Doc.List {
		match := docLineRE.FindStringSubmatch(comment.Text)
		if match == nil {
			fnComments = append(fnComments, comment)
			continue
		}
		matchesLink := false
		for link := range linksMap {
			if sameDocLink(match[2], link) {
				matchesLink = true
				delete(linksMap, link)
				break
			}
		}
		if matchesLink {
			fnComments = append(fnComments, comment)
		}
	}

	// remove trailing empty lines
	for len(fnComments) > 0 {
		if !emptyLineRE.MatchString(fnComments[len(fnComments)-1].Text) {
			break
		}
		fnComments = fnComments[:len(fnComments)-1]
	}

	// add an empty line before adding doc links
	if len(linksMap) > 0 &&
		len(fnComments) > 0 &&
		!emptyLineRE.MatchString(fnComments[len(fnComments)-1].Text) {
		fnComments = append(fnComments, &ast.Comment{Text: "//"})
	}

	docLinks := maps.Keys(linksMap)
	sort.Strings(docLinks)

	for _, dl := range docLinks {
		fnComments = append(
			fnComments,
			&ast.Comment{
				Text: "// GitHub API docs: " + normalizeDocURLPath(dl),
			},
		)
	}
	fn.Doc.List = fnComments
	return true
}

const docURLPrefix = "https://docs.github.com/rest/"

var docURLPrefixRE = regexp.MustCompile(`^https://docs\.github\.com.*/rest/`)

func normalizeDocURLPath(u string) string {
	u = strings.Replace(u, "/en/", "/", 1)
	pre := docURLPrefixRE.FindString(u)
	if pre == "" {
		return u
	}
	if strings.Contains(u, "docs.github.com/enterprise-cloud@latest/") {
		// remove unsightly double slash
		// https://docs.github.com/enterprise-cloud@latest/
		return strings.ReplaceAll(
			u,
			"docs.github.com/enterprise-cloud@latest//",
			"docs.github.com/enterprise-cloud@latest/",
		)
	}
	if strings.Contains(u, "docs.github.com/enterprise-server") {
		return u
	}
	return docURLPrefix + strings.TrimPrefix(u, pre)
}

// sameDocLink returns true if the two doc links are going to end up rendering the same page pointed
// to the same section.
//
// If a url path starts with *./rest/ it ignores query parameters and everything before /rest/ when
// making the comparison.
func sameDocLink(left, right string) bool {
	if !docURLPrefixRE.MatchString(left) ||
		!docURLPrefixRE.MatchString(right) {
		return left == right
	}
	left = stripURLQuery(normalizeDocURLPath(left))
	right = stripURLQuery(normalizeDocURLPath(right))
	return left == right
}

func stripURLQuery(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return u
	}
	p.RawQuery = ""
	return p.String()
}
