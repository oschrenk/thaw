// Package lock reads a flake.lock and resolves its inputs as a graph.
//
// Node names are not input names. A flake with three distinct nixpkgs holds
// nodes "nixpkgs", "nixpkgs_2" and "nixpkgs_3", and which one an input means
// is only knowable by following root.inputs. Looking a node up by its name is
// the bug this package exists to prevent.
package lock

import (
	"encoding/json"
	"fmt"
	"os"
)

const maxDepth = 100

// Ref is an entry in a node's inputs. It is either a node key, or a path of
// input names to follow from the root.
type Ref struct {
	Key    string
	Follow []string
}

func (r *Ref) UnmarshalJSON(b []byte) error {
	var key string
	if err := json.Unmarshal(b, &key); err == nil {
		r.Key = key
		return nil
	}
	var follow []string
	if err := json.Unmarshal(b, &follow); err != nil {
		return fmt.Errorf("input ref is neither a node key nor a follows path: %s", b)
	}
	r.Follow = follow
	return nil
}

type Locked struct {
	Type    string `json:"type"`
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	Ref     string `json:"ref"`
	Rev     string `json:"rev"`
	NarHash string `json:"narHash"`
	URL     string `json:"url"`
}

type Node struct {
	Inputs   map[string]Ref `json:"inputs"`
	Locked   *Locked        `json:"locked"`
	Original *Locked        `json:"original"`
}

type Lock struct {
	Version int             `json:"version"`
	Root    string          `json:"root"`
	Nodes   map[string]Node `json:"nodes"`
}

func Load(path string) (*Lock, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

func Parse(b []byte) (*Lock, error) {
	var l Lock
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, fmt.Errorf("parse flake.lock: %w", err)
	}
	if l.Root == "" {
		return nil, fmt.Errorf("flake.lock names no root node")
	}
	if _, ok := l.Nodes[l.Root]; !ok {
		return nil, fmt.Errorf("root node %q is absent from nodes", l.Root)
	}
	return &l, nil
}

// Resolve walks a path of input names from the root and returns the node key
// it lands on. Resolve("nixpkgs") answers what the root means by nixpkgs,
// which is rarely the node called "nixpkgs".
func (l *Lock) Resolve(path ...string) (string, error) {
	return l.resolveFrom(l.Root, path, 0)
}

func (l *Lock) resolveFrom(start string, path []string, depth int) (string, error) {
	if depth > maxDepth {
		return "", fmt.Errorf("input path %v exceeds %d hops, the lock is cyclic", path, maxDepth)
	}
	key := start
	for _, name := range path {
		node, ok := l.Nodes[key]
		if !ok {
			return "", fmt.Errorf("node %q is absent from nodes", key)
		}
		ref, ok := node.Inputs[name]
		if !ok {
			return "", fmt.Errorf("node %q has no input %q", key, name)
		}
		if ref.Follow != nil {
			resolved, err := l.resolveFrom(l.Root, ref.Follow, depth+1)
			if err != nil {
				return "", fmt.Errorf("following %v from %q: %w", ref.Follow, key, err)
			}
			key = resolved
			continue
		}
		if ref.Key == "" {
			return "", fmt.Errorf("input %q of node %q names no node", name, key)
		}
		key = ref.Key
	}
	return key, nil
}

// Node returns the node a path of input names resolves to.
func (l *Lock) Node(path ...string) (Node, error) {
	key, err := l.Resolve(path...)
	if err != nil {
		return Node{}, err
	}
	return l.Nodes[key], nil
}

// Rev returns the locked revision a path of input names resolves to.
func (l *Lock) Rev(path ...string) (string, error) {
	node, err := l.Node(path...)
	if err != nil {
		return "", err
	}
	if node.Locked == nil {
		return "", fmt.Errorf("input %v has no locked entry", path)
	}
	return node.Locked.Rev, nil
}

// RootInputs maps every input name the root declares to the node key it means.
func (l *Lock) RootInputs() (map[string]string, error) {
	out := map[string]string{}
	for name := range l.Nodes[l.Root].Inputs {
		key, err := l.Resolve(name)
		if err != nil {
			return nil, err
		}
		out[name] = key
	}
	return out, nil
}
