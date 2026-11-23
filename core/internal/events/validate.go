package events

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const schemaID = "https://knowgap-lab.example/contracts/events.schema.json"

var (
	schemaOnce sync.Once
	schemaDoc  *jsonschema.Schema
	schemaErr  error
)

// Validate проверяет один документ по docs/contracts/events.schema.json.
func Validate(doc []byte) error {
	schema, err := compiledSchema()
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return err
	}
	return schema.Validate(instance)
}

func compiledSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		schemaDoc, schemaErr = loadSchema()
	})
	return schemaDoc, schemaErr
}

func loadSchema() (*jsonschema.Schema, error) {
	path, err := schemaPath()
	if err != nil {
		return nil, err
	}
	// Путь собран обходом каталогов до фиксированного файла контракта, не из запроса.
	raw, err := os.ReadFile(path) //nolint:gosec // G304: docs/contracts/events.schema.json от корня репозитория
	if err != nil {
		return nil, err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaID, doc); err != nil {
		return nil, err
	}
	return compiler.Compile(schemaID)
}

func schemaPath() (string, error) {
	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		starts = append(starts, filepath.Dir(file))
	}
	for _, start := range starts {
		dir := start
		for {
			candidate := filepath.Join(dir, "docs", "contracts", "events.schema.json")
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				return candidate, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", errors.New("events.schema.json не найден")
}
