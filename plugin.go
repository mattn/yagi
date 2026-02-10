package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/traefik/yaegi/stdlib/unrestricted"
)

var (
	hostSymbols = interp.Exports{
		"hostapi/hostapi": map[string]reflect.Value{
			"FetchURL":      reflect.ValueOf(fetchURL),
			"HTMLToText":    reflect.ValueOf(htmlToText),
			"WebSocketSend": reflect.ValueOf(webSocketSend),
			"SaveMemory":    reflect.ValueOf(saveMemoryEntry),
			"GetMemory":     reflect.ValueOf(getMemoryEntry),
			"DeleteMemory":  reflect.ValueOf(deleteMemoryEntry),
			"ListMemory":    reflect.ValueOf(listMemoryEntries),
		},
	}
	skipApproval    bool
	pluginWorkDir   string
	pluginApprovals *approvalRecord
	pluginConfigDir string
)

type approvalRecord struct {
	Directories map[string][]string `json:"directories"` // directory -> plugin names
}

func loadApprovalRecords(configDir string) (*approvalRecord, error) {
	path := filepath.Join(configDir, "approved_plugins.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &approvalRecord{Directories: make(map[string][]string)}, nil
		}
		return nil, err
	}
	var record approvalRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	if record.Directories == nil {
		record.Directories = make(map[string][]string)
	}
	return &record, nil
}

func saveApprovalRecords(configDir string, record *approvalRecord) error {
	path := filepath.Join(configDir, "approved_plugins.json")
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func computeHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func requestApproval(pluginName, workDir, arguments string) bool {
	fmt.Fprintf(os.Stderr, "\n[WARNING] Plugin requires approval\n")
	fmt.Fprintf(os.Stderr, "  Plugin: %s\n", pluginName)
	fmt.Fprintf(os.Stderr, "  Working directory: %s\n", workDir)
	fmt.Fprintf(os.Stderr, "  Arguments: %s\n", arguments)
	fmt.Fprintf(os.Stderr, "This plugin uses unrestricted API and may perform dangerous operations.\n")

	response, err := readFromTTY("Allow this plugin for this directory? [y/N]: ")
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

func isPluginApproved(approvals *approvalRecord, workDir, pluginName string) bool {
	if plugins, exists := approvals.Directories[workDir]; exists {
		for _, name := range plugins {
			if name == pluginName {
				return true
			}
		}
	}
	return false
}

func addPluginApproval(approvals *approvalRecord, workDir, pluginName string) {
	plugins := approvals.Directories[workDir]
	for _, name := range plugins {
		if name == pluginName {
			return // already exists
		}
	}
	approvals.Directories[workDir] = append(plugins, pluginName)
}

func removePluginApproval(approvals *approvalRecord, workDir, pluginName string) bool {
	plugins, exists := approvals.Directories[workDir]
	if !exists {
		return false
	}
	for i, name := range plugins {
		if name == pluginName {
			approvals.Directories[workDir] = append(plugins[:i], plugins[i+1:]...)
			if len(approvals.Directories[workDir]) == 0 {
				delete(approvals.Directories, workDir)
			}
			return true
		}
	}
	return false
}

func removeAllPluginApprovals(approvals *approvalRecord, workDir string) int {
	plugins, exists := approvals.Directories[workDir]
	if !exists {
		return 0
	}
	count := len(plugins)
	delete(approvals.Directories, workDir)
	return count
}

func listApprovedPlugins(approvals *approvalRecord, workDir string) []string {
	if plugins, exists := approvals.Directories[workDir]; exists {
		return plugins
	}
	return nil
}

func loadPlugins(dir, configDir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	approvals, err := loadApprovalRecords(configDir)
	if err != nil {
		return fmt.Errorf("failed to load approval records: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := loadPlugin(path, workDir, configDir, approvals); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load plugin %s: %v\n", path, err)
		}
	}

	return nil
}

func loadPlugin(path, workDir, configDir string, approvals *approvalRecord) error {
	// Store for later use in executeTool
	pluginWorkDir = workDir
	pluginConfigDir = configDir
	pluginApprovals = approvals

	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)
	i.Use(unrestricted.Symbols)
	i.Use(hostSymbols)

	_, err = i.Eval(string(src))
	if err != nil {
		return fmt.Errorf("eval: %w", err)
	}

	toolVal, err := i.Eval("tool.Tool")
	if err != nil {
		return fmt.Errorf("tool.Tool not found: %w", err)
	}

	v := toolVal.Interface()
	rv := reflect.ValueOf(v)

	nameField := rv.FieldByName("Name")
	if !nameField.IsValid() || nameField.Kind() != reflect.String {
		return fmt.Errorf("Tool.Name field not found or not a string")
	}
	name := nameField.String()

	descField := rv.FieldByName("Description")
	if !descField.IsValid() || descField.Kind() != reflect.String {
		return fmt.Errorf("Tool.Description field not found or not a string")
	}
	description := descField.String()

	paramsField := rv.FieldByName("Parameters")
	if !paramsField.IsValid() || paramsField.Kind() != reflect.String {
		return fmt.Errorf("Tool.Parameters field not found or not a string")
	}
	parameters := paramsField.String()

	runField := rv.FieldByName("Run")
	if !runField.IsValid() || runField.Kind() != reflect.Func {
		return fmt.Errorf("Tool.Run field not found or not a function")
	}

	runFn := convertRunFunc(runField)
	registerTool(name, description, json.RawMessage(parameters), runFn, false)
	if verbose {
		fmt.Fprintf(os.Stderr, "Loaded plugin: %s\n", name)
	}
	return nil
}

func convertRunFunc(runVal reflect.Value) func(context.Context, string) (string, error) {
	return func(ctx context.Context, args string) (string, error) {
		results := runVal.Call([]reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(args),
		})
		if len(results) >= 2 {
			if err, ok := results[1].Interface().(error); ok && err != nil {
				return "", err
			}
			return results[0].Interface().(string), nil
		}
		if len(results) > 0 {
			return results[0].Interface().(string), nil
		}
		return "", nil
	}
}
