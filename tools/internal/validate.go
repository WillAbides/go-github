package internal

import (
	"fmt"
	"slices"
	"sort"
)

// ValidateMetadata returns a list of issues with the metadata file. An error means
// there was an error validating the file, not that there are issues with the file.
func ValidateMetadata(dir string, meta *Metadata) ([]string, error) {
	undocumentedMethods, err := GetUndocumentedMethods(dir, meta)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, m := range undocumentedMethods {
		msg := fmt.Sprintf("Undocumented method %s. Please add it to metadata.", m.Name())
		result = append(result, msg)
	}
	metaMethodMap := map[string]bool{}
	for _, m := range meta.UndocumentedMethods {
		metaMethodMap[m] = true
	}
	for _, op := range meta.Operations {
		for _, m := range op.GoMethods {
			metaMethodMap[m] = true
		}
	}
	var metaMethods []string
	for m := range metaMethodMap {
		metaMethods = append(metaMethods, m)
	}
	sort.Strings(metaMethods)
	serviceMethods, err := GetServiceMethods(dir)
	if err != nil {
		return nil, err
	}
	if len(serviceMethods) == 0 {
		return nil, fmt.Errorf("no service methods found in %s", dir)
	}
	smNames := map[string]bool{}
	for _, m := range serviceMethods {
		smNames[m.Name()] = true
	}
	for _, m := range metaMethods {
		if !smNames[m] {
			msg := fmt.Sprintf("Method %s in metadata does not exist in github package.", m)
			result = append(result, msg)
		}
	}
	return result, nil
}

// GetUndocumentedMethods returns a list of methods that are not mapped to any operation in metadata.yaml
func GetUndocumentedMethods(dir string, metadata *Metadata) ([]*ServiceMethod, error) {
	var result []*ServiceMethod
	methods, err := GetServiceMethods(dir)
	if err != nil {
		return nil, err
	}
	for _, method := range methods {
		ops := metadata.OperationsForMethod(method.Name())
		if len(ops) > 0 {
			continue
		}
		if slices.Contains(metadata.UndocumentedMethods, method.Name()) {
			continue
		}
		result = append(result, method)
	}
	return result, nil
}

// MissingMethods returns the set from methods that do not exist in the github package.
func MissingMethods(dir string, methods []string) ([]string, error) {
	var result []string
	existingMap := map[string]bool{}
	sm, err := GetServiceMethods(dir)
	if err != nil {
		return nil, err
	}
	for _, method := range sm {
		existingMap[method.Name()] = true
	}
	for _, m := range methods {
		if !existingMap[m] {
			result = append(result, m)
		}
	}
	return result, nil
}
