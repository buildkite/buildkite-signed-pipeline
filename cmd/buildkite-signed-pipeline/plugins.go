package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
)

var (
	// 'official-plugin' and 'official-plugin#v2'
	officialPluginRegex = regexp.MustCompile(`^([A-Za-z0-9-]+)(#.+)?$`)
	// 'some-org/some-plugin' and 'some-org/some-plugin#v2'
	githubPluginRegex = regexp.MustCompile(`^([A-Za-z0-9-]+\/[A-Za-z0-9-]+)(#.+)?$`)
)

type Plugin struct {
	Name   string
	Params map[string]any
}

func NewPluginFromReference(item any) (*Plugin, error) {
	switch i := item.(type) {
	// plugin references that are just a plugin name, e.g. docker#v1.2.3
	case string:
		return &Plugin{i, nil}, nil
	// plugin references that are a name and a set of settings
	case map[string]any:
		for name, settings := range i {
			// note that x.(T) is avoided here as settings may be null in the case
			// of plugins without parameters
			parameters, _ := settings.(map[string]any)
			return &Plugin{name, parameters}, nil
		}
	}
	return nil, fmt.Errorf("unknown plugin reference type %T", item)
}

func (p Plugin) Repository() string {
	if m := officialPluginRegex.FindStringSubmatch(p.Name); len(m) == 3 {
		return fmt.Sprintf(`github.com/buildkite-plugins/%s-buildkite-plugin%s`, m[1], m[2])
	}
	if m := githubPluginRegex.FindStringSubmatch(p.Name); len(m) == 3 {
		return fmt.Sprintf(`github.com/%s-buildkite-plugin%s`, m[1], m[2])
	}
	return p.Name
}

// The bootstrap expects an array of plugins like [{"plugin1#v1.0.0":{...}}, {"plugin2#v1.0.0":{...}}]
func marshalPlugins(plugins []Plugin) (string, error) {
	var p []any
	for _, plugin := range plugins {
		p = append(p, map[string]any{
			plugin.Repository(): plugin.Params,
		})
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", nil
	}
	return string(b), nil
}

func getPluginPair(pluginReference map[string]any) (string, any) {
	for k, v := range pluginReference {
		return k, v
	}
	return "", nil
}

func canonicalisePluginJSON(pluginJSON string) (string, error) {
	// plugin JSON is of the form [{"plugin-ref#version":{settings}},{"plugin-ref2#version":null}]
	// https://golang.org/pkg/encoding/json/#Marshal provides consistent ordering of JSON
	// unmarshal and remarshal to ensure this ordering is the same as extraction
	var plugins []map[string]any
	if err := json.Unmarshal([]byte(pluginJSON), &plugins); err != nil {
		return "", err
	}

	// sort by the plugin ref
	sort.Slice(plugins, func(i, j int) bool {
		thisName, _ := getPluginPair(plugins[i])
		otherName, _ := getPluginPair(plugins[j])
		return thisName < otherName
	})

	pluginBytes, err := json.Marshal(plugins)
	if err != nil {
		return "", err
	}
	return string(pluginBytes), nil
}
