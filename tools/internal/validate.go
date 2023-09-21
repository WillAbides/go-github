// Copyright 2023 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"
	"fmt"
)

// ValidateMetadata returns a list of issues with the metadata file. An error means
// there was an error validating the file, not that there are issues with the file.
//
// validations:
//   - Methods in the github package must exist in metadata.yaml
//   - Methods in metadata.yaml must exist in github package
//   - Methods in metadata.yaml must have unique names
//   - Methods in metadata.yaml must have at least one operation
//   - Methods in metadata.yaml may not have duplicate operations
//   - Methods in metadata.yaml must use the canonical operation name
//   - All operations mapped from a method must exist in either ManualOps or OpenAPIOps
//   - No operations are duplicated between ManualOps and OpenAPIOps
//   - All operations in OverrideOps must exist in either ManualOps or OpenAPIOps
func ValidateMetadata(dir string, meta *Metadata) ([]string, error) {
	serviceMethods, err := getServiceMethods(dir)
	if err != nil {
		return nil, err
	}
	var result []string
	result = validateServiceMethodsExist(result, meta, serviceMethods)
	result = validateMetadataMethods(result, meta, serviceMethods)
	result = validateOperations(result, meta)
	return result, nil
}

// ValidateGitCommit validates that building meta.OpenapiOps from the commit at meta.GitCommit
// results in the same operations as meta.OpenapiOps.
func ValidateGitCommit(ctx context.Context, client contentsClient, meta *Metadata) (string, error) {
	ops, err := getOpsFromGithub(ctx, client, meta.GitCommit)
	if err != nil {
		return "", err
	}
	if !operationsEqual(ops, meta.OpenapiOps) {
		msg := fmt.Sprintf("openapi_operations does not match operations from git commit %s", meta.GitCommit)
		return msg, nil
	}
	return "", nil
}

func validateMetadataMethods(result []string, meta *Metadata, serviceMethods []string) []string {
	smLookup := map[string]bool{}
	for _, method := range serviceMethods {
		smLookup[method] = true
	}
	seenMethods := map[string]bool{}
	for _, method := range meta.Methods {
		if seenMethods[method.Name] {
			msg := fmt.Sprintf("Method %s is duplicated in metadata.yaml.", method.Name)
			result = append(result, msg)
			continue
		}
		seenMethods[method.Name] = true
		if !smLookup[method.Name] {
			msg := fmt.Sprintf("Method %s in metadata.yaml does not exist in github package.", method.Name)
			result = append(result, msg)
		}
		result = validateMetaMethodOperations(result, meta, method)
	}
	return result
}

func validateMetaMethodOperations(result []string, meta *Metadata, method *Method) []string {
	if len(method.OpNames) == 0 {
		msg := fmt.Sprintf("Method %s in metadata.yaml does not have any operations.", method.Name)
		result = append(result, msg)
	}
	seenOps := map[string]bool{}
	for _, opName := range method.OpNames {
		if seenOps[opName] {
			msg := fmt.Sprintf("Method %s in metadata.yaml has duplicate operation: %s.", method.Name, opName)
			result = append(result, msg)
		}
		seenOps[opName] = true
		if meta.getOperation(opName) != nil {
			continue
		}
		normalizedMatch := meta.getOperationsWithNormalizedName(opName)
		if len(normalizedMatch) > 0 {
			msg := fmt.Sprintf("Method %s has operation which is does not use the canonical name. You may be able to automatically fix this by running 'script/metadata.sh canonize': %s.", method.Name, opName)
			result = append(result, msg)
			continue
		}
		msg := fmt.Sprintf("Method %s has operation which is not defined in metadata.yaml: %s.", method.Name, opName)
		result = append(result, msg)
	}
	return result
}

func validateServiceMethodsExist(result []string, meta *Metadata, serviceMethods []string) []string {
	for _, method := range serviceMethods {
		if meta.getMethod(method) == nil {
			msg := fmt.Sprintf("Method %s does not exist in metadata.yaml. Please add it.", method)
			result = append(result, msg)
		}
	}
	return result
}

func validateOperations(result []string, meta *Metadata) []string {
	names := map[string]bool{}
	openapiNames := map[string]bool{}
	overrideNames := map[string]bool{}
	for _, op := range meta.OpenapiOps {
		if openapiNames[op.Name] {
			msg := fmt.Sprintf("Name duplicated in openapi_operations: %s", op.Name)
			result = append(result, msg)
		}
		openapiNames[op.Name] = true
	}
	for _, op := range meta.ManualOps {
		if names[op.Name] {
			msg := fmt.Sprintf("Name duplicated in operations: %s", op.Name)
			result = append(result, msg)
		}
		names[op.Name] = true
		if openapiNames[op.Name] {
			msg := fmt.Sprintf("Name exists in both operations and openapi_operations: %s", op.Name)
			result = append(result, msg)
		}
	}
	for _, op := range meta.OverrideOps {
		if overrideNames[op.Name] {
			msg := fmt.Sprintf("Name duplicated in override_operations: %s", op.Name)
			result = append(result, msg)
		}
		overrideNames[op.Name] = true
		if !names[op.Name] && !openapiNames[op.Name] {
			msg := fmt.Sprintf("Name in override_operations does not exist in operations or openapi_operations: %s", op.Name)
			result = append(result, msg)
		}
	}
	return result
}
