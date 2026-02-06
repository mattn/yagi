package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

var hostSymbols = interp.Exports{
	"hostapi/hostapi": map[string]reflect.Value{
		"NostrFetchNotes": reflect.ValueOf(nostrFetchNotes),
	},
}

func loadPlugins(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := loadPlugin(path); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load plugin %s: %v\n", path, err)
		}
	}
	return nil
}

func loadPlugin(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)
	i.Use(hostSymbols)

	_, err = i.Eval(string(src))
	if err != nil {
		return fmt.Errorf("eval: %w", err)
	}

	nameVal, err := i.Eval("tool.Name")
	if err != nil {
		return fmt.Errorf("Name not found: %w", err)
	}
	name := nameVal.Interface().(string)

	descVal, err := i.Eval("tool.Description")
	if err != nil {
		return fmt.Errorf("Description not found: %w", err)
	}
	description := descVal.Interface().(string)

	paramsVal, err := i.Eval("tool.Parameters")
	if err != nil {
		return fmt.Errorf("Parameters not found: %w", err)
	}
	parameters := paramsVal.Interface().(string)

	runVal, err := i.Eval("tool.Run")
	if err != nil {
		return fmt.Errorf("Run not found: %w", err)
	}

	runFn, ok := runVal.Interface().(func(string) string)
	if !ok {
		if runVal.Kind() == reflect.Func {
			runFn = func(args string) string {
				results := runVal.Call([]reflect.Value{reflect.ValueOf(args)})
				if len(results) > 0 {
					return results[0].Interface().(string)
				}
				return ""
			}
		} else {
			return fmt.Errorf("Run is not a function")
		}
	}

	registerTool(name, description, json.RawMessage(parameters), runFn)
	if !quiet {
		fmt.Fprintf(os.Stderr, "Loaded plugin: %s\n", name)
	}
	return nil
}
