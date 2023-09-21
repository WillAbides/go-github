// Copyright 2023 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"bytes"
	"context"
	"fmt"
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

	"gopkg.in/yaml.v3"
)

type Operation struct {
	Name             string   `yaml:"name,omitempty" json:"name,omitempty"`
	DocumentationURL string   `yaml:"documentation_url,omitempty" json:"documentation_url,omitempty"`
	OpenAPIFiles     []string `yaml:"openapi_files,omitempty" json:"openapi_files,omitempty"`
}

func (o *Operation) equal(other *Operation) bool {
	if o.Name != other.Name || o.DocumentationURL != other.DocumentationURL {
		return false
	}
	if len(o.OpenAPIFiles) != len(other.OpenAPIFiles) {
		return false
	}
	for i := range o.OpenAPIFiles {
		if o.OpenAPIFiles[i] != other.OpenAPIFiles[i] {
			return false
		}
	}
	return true
}

func (o *Operation) clone() *Operation {
	return &Operation{
		Name:             o.Name,
		DocumentationURL: o.DocumentationURL,
		OpenAPIFiles:     append([]string{}, o.OpenAPIFiles...),
	}
}

func operationsEqual(a, b []*Operation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].equal(b[i]) {
			return false
		}
	}
	return true
}

func sortOperations(ops []*Operation) {
	sort.Slice(ops, func(i, j int) bool {
		leftVerb, leftURL := parseOpName(ops[i].Name)
		rightVerb, rightURL := parseOpName(ops[j].Name)
		if leftURL != rightURL {
			return leftURL < rightURL
		}
		return leftVerb < rightVerb
	})
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

func normalizedOpName(name string) string {
	verb, u := parseOpName(name)
	return verb + " " + normalizedURL(u)
}

func parseOpName(id string) (verb, url string) {
	verb, url, _ = strings.Cut(id, " ")
	return verb, url
}

type Method struct {
	Name    string   `yaml:"name" json:"name"`
	OpNames []string `yaml:"operations,omitempty" json:"operations,omitempty"`
}

type Metadata struct {
	Methods     []*Method    `yaml:"methods,omitempty"`
	ManualOps   []*Operation `yaml:"operations"`
	OverrideOps []*Operation `yaml:"operation_overrides"`
	GitCommit   string       `yaml:"openapi_commit"`
	OpenapiOps  []*Operation `yaml:"openapi_operations"`

	mu          sync.Mutex
	resolvedOps map[string]*Operation
}

func (m *Metadata) resolve() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.resolvedOps != nil {
		return
	}
	m.resolvedOps = map[string]*Operation{}
	for _, op := range m.OpenapiOps {
		m.resolvedOps[op.Name] = op.clone()
	}
	for _, op := range m.ManualOps {
		m.resolvedOps[op.Name] = op.clone()
	}
	for _, override := range m.OverrideOps {
		override = override.clone()
		_, ok := m.resolvedOps[override.Name]
		if !ok {
			m.resolvedOps[override.Name] = override
		}
		if override.DocumentationURL != "" {
			m.resolvedOps[override.Name].DocumentationURL = override.DocumentationURL
		}
		if len(override.OpenAPIFiles) > 0 {
			m.resolvedOps[override.Name].OpenAPIFiles = override.OpenAPIFiles
		}
	}
}

func (m *Metadata) Operations() []*Operation {
	m.resolve()
	ops := make([]*Operation, 0, len(m.resolvedOps))
	for _, op := range m.resolvedOps {
		ops = append(ops, op)
	}
	sortOperations(ops)
	return ops
}

func LoadMetadataFile(filename string) (*Metadata, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var meta Metadata
	err = yaml.Unmarshal(b, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func (m *Metadata) SaveFile(filename string) (errOut error) {
	sortOperations(m.ManualOps)
	sortOperations(m.OverrideOps)
	sortOperations(m.OpenapiOps)
	sort.Slice(m.Methods, func(i, j int) bool {
		return m.Methods[i].Name < m.Methods[j].Name
	})
	for _, method := range m.Methods {
		sort.Strings(method.OpNames)
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

func addOperation(ops []*Operation, filename, opName, docURL string) []*Operation {
	for _, op := range ops {
		if opName != op.Name {
			continue
		}
		if len(op.OpenAPIFiles) == 0 {
			op.OpenAPIFiles = append(op.OpenAPIFiles, filename)
			op.DocumentationURL = docURL
			return ops
		}
		// just append to files, but only add the first ghes file
		if !strings.Contains(filename, "/ghes") {
			op.OpenAPIFiles = append(op.OpenAPIFiles, filename)
			return ops
		}
		for _, f := range op.OpenAPIFiles {
			if strings.Contains(f, "/ghes") {
				return ops
			}
		}
		op.OpenAPIFiles = append(op.OpenAPIFiles, filename)
		return ops
	}
	return append(ops, &Operation{
		Name:             opName,
		OpenAPIFiles:     []string{filename},
		DocumentationURL: docURL,
	})
}

// OperationMethods returns a list methods that are mapped to the given operation id.
func (m *Metadata) OperationMethods(opName string) []string {
	var methods []string
	for _, method := range m.Methods {
		for _, methodOpName := range method.OpNames {
			if methodOpName == opName {
				methods = append(methods, method.Name)
			}
		}
	}
	return methods
}

func (m *Metadata) getOperation(name string) *Operation {
	m.resolve()
	return m.resolvedOps[name]
}

func (m *Metadata) getOperationsWithNormalizedName(name string) []*Operation {
	m.resolve()
	var result []*Operation
	norm := normalizedOpName(name)
	for n := range m.resolvedOps {
		if normalizedOpName(n) == norm {
			result = append(result, m.resolvedOps[n])
		}
	}
	sortOperations(result)
	return result
}

func (m *Metadata) getMethod(name string) *Method {
	for _, method := range m.Methods {
		if method.Name == name {
			return method
		}
	}
	return nil
}

func (m *Metadata) operationsForMethod(methodName string) []*Operation {
	method := m.getMethod(methodName)
	if method == nil {
		return nil
	}
	var operations []*Operation
	for _, name := range method.OpNames {
		op := m.getOperation(name)
		if op != nil {
			operations = append(operations, op)
		}
	}
	sortOperations(operations)
	return operations
}

func (m *Metadata) CanonizeMethodOperations() error {
	for _, method := range m.Methods {
		for i := range method.OpNames {
			opName := method.OpNames[i]
			if m.getOperation(opName) != nil {
				continue
			}
			ops := m.getOperationsWithNormalizedName(opName)
			switch len(ops) {
			case 0:
				return fmt.Errorf("method %q has an operation that can not be canonized to any defined name: %s", method.Name, opName)
			case 1:
				method.OpNames[i] = ops[0].Name
			default:
				candidateList := ""
				for _, op := range ops {
					candidateList += "\n    " + op.Name
				}
				return fmt.Errorf("method %q has an operation that can be canonized to multiple defined names:\n  operation: %s\n  matches: %s", method.Name, opName, candidateList)
			}
		}
	}
	return nil
}

func (m *Metadata) UpdateFromGithub(ctx context.Context, client contentsClient, ref string) error {
	commit, resp, err := client.GetCommit(ctx, descriptionsOwnerName, descriptionsRepoName, ref, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected status code: %s", resp.Status)
	}
	ops, err := getOpsFromGithub(ctx, client, ref)
	if err != nil {
		return err
	}
	if !operationsEqual(m.OpenapiOps, ops) {
		m.OpenapiOps = ops
		m.GitCommit = commit.GetSHA()
	}
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
	if !ok {
		return true
	}
	sm := serviceMethodFromNode(n)
	if sm == "" {
		return true
	}

	linksMap := map[string]struct{}{}
	undocMap := map[string]bool{}
	ops := metadata.operationsForMethod(sm)
	for _, op := range ops {
		if op.DocumentationURL == "" {
			undocMap[op.Name] = true
			continue
		}
		linksMap[op.DocumentationURL] = struct{}{}
	}
	var undocumentedOps []string
	for op := range undocMap {
		undocumentedOps = append(undocumentedOps, op)
	}
	sort.Strings(undocumentedOps)

	// create copy of comment group with non-matching doc links removed
	if fn.Doc == nil {
		fn.Doc = &ast.CommentGroup{}
	}
	fnComments := make([]*ast.Comment, 0, len(fn.Doc.List))
	skipSpacer := false
	for _, comment := range fn.Doc.List {
		if strings.Contains(comment.Text, "uses the undocumented GitHub API endpoint") {
			skipSpacer = true
			continue
		}
		match := docLineRE.FindStringSubmatch(comment.Text)
		if match == nil {
			fnComments = append(fnComments, comment)
			continue
		}
		matchesLink := false
		for link := range linksMap {
			if sameDocLink(match[2], link) {
				matchesLink = true
				skipSpacer = true
				delete(linksMap, link)
				break
			}
		}
		if matchesLink {
			fnComments = append(fnComments, comment)
		}
	}

	// add an empty line before adding doc links
	if len(linksMap)+len(undocumentedOps) > 0 && !skipSpacer &&
		!emptyLineRE.MatchString(fnComments[len(fnComments)-1].Text) {
		fnComments = append(fnComments, &ast.Comment{Text: "//"})
	}

	var docLinks []string
	for link := range linksMap {
		docLinks = append(docLinks, link)
	}
	sort.Strings(docLinks)

	for _, dl := range docLinks {
		fnComments = append(
			fnComments,
			&ast.Comment{
				Text: "// GitHub API docs: " + normalizeDocURLPath(dl),
			},
		)
	}
	_, methodName, _ := strings.Cut(sm, ".")
	for _, opName := range undocumentedOps {
		line := fmt.Sprintf("// Note: %s uses the undocumented GitHub API endpoint %q.", methodName, opName)
		fnComments = append(fnComments, &ast.Comment{Text: line})
	}
	if len(docLinks)+len(undocumentedOps) > 0 {
		fn.Doc.List = fnComments
		return true
	}
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
