package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractPythonRelationships_Imports(t *testing.T) {
	source := `
import os
import sys
from pathlib import Path
from . import local_module
from ..parent import something
`

	p, err := NewParser(LanguagePython)
	require.NoError(t, err)

	result, err := p.ParseWithRelationships([]byte(source), "test.py")
	require.NoError(t, err)

	imports := filterRelsByKind(result.Relationships, RelationshipImports)
	assert.GreaterOrEqual(t, len(imports), 4)

	// Check specific imports
	paths := extractTargetPaths(imports)
	assert.Contains(t, paths, "os")
	assert.Contains(t, paths, "sys")
	assert.Contains(t, paths, "pathlib")
}

func TestExtractPythonRelationships_Calls(t *testing.T) {
	source := `
def outer():
    inner()
    helper.process()

def inner():
    pass
`

	p, err := NewParser(LanguagePython)
	require.NoError(t, err)

	result, err := p.ParseWithRelationships([]byte(source), "test.py")
	require.NoError(t, err)

	calls := filterRelsByKind(result.Relationships, RelationshipCalls)
	assert.GreaterOrEqual(t, len(calls), 1)

	// outer calls inner
	found := false
	for _, c := range calls {
		if c.SourceName == "outer" && c.TargetName == "inner" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected outer -> inner call relationship")
}

func TestExtractPythonRelationships_Extends(t *testing.T) {
	source := `
class Base:
    pass

class Child(Base):
    pass

class MultiChild(Base, Mixin):
    pass
`

	p, err := NewParser(LanguagePython)
	require.NoError(t, err)

	result, err := p.ParseWithRelationships([]byte(source), "test.py")
	require.NoError(t, err)

	extends := filterRelsByKind(result.Relationships, RelationshipExtends)
	assert.GreaterOrEqual(t, len(extends), 2)

	// Child extends Base
	found := false
	for _, e := range extends {
		if e.SourceName == "Child" && e.TargetName == "Base" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected Child extends Base relationship")
}

func TestExtractJavaScriptRelationships_Imports(t *testing.T) {
	source := `
import React from 'react';
import { useState, useEffect } from 'react';
const path = require('path');
`

	p, err := NewParser(LanguageJavaScript)
	require.NoError(t, err)

	result, err := p.ParseWithRelationships([]byte(source), "test.js")
	require.NoError(t, err)

	imports := filterRelsByKind(result.Relationships, RelationshipImports)
	assert.GreaterOrEqual(t, len(imports), 2)

	paths := extractTargetPaths(imports)
	assert.Contains(t, paths, "react")
	assert.Contains(t, paths, "path")
}

func TestExtractJavaScriptRelationships_Extends(t *testing.T) {
	source := `
class Component extends React.Component {
    render() {
        return null;
    }
}

class UserService extends BaseService {
    getUser(id) {
        return this.fetch(id);
    }
}
`

	p, err := NewParser(LanguageJavaScript)
	require.NoError(t, err)

	result, err := p.ParseWithRelationships([]byte(source), "test.js")
	require.NoError(t, err)

	extends := filterRelsByKind(result.Relationships, RelationshipExtends)
	assert.GreaterOrEqual(t, len(extends), 1)

	// Look for class extends relationship
	found := false
	for _, e := range extends {
		if e.SourceName == "UserService" && e.TargetName == "BaseService" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected UserService extends BaseService relationship")
}

func TestExtractJavaScriptRelationships_Calls(t *testing.T) {
	source := `
function main() {
    helper();
    utils.process();
}

function helper() {
    console.log('test');
}
`

	p, err := NewParser(LanguageJavaScript)
	require.NoError(t, err)

	result, err := p.ParseWithRelationships([]byte(source), "test.js")
	require.NoError(t, err)

	calls := filterRelsByKind(result.Relationships, RelationshipCalls)
	assert.GreaterOrEqual(t, len(calls), 1)

	// main calls helper
	found := false
	for _, c := range calls {
		if c.SourceName == "main" && c.TargetName == "helper" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected main -> helper call relationship")
}

// Helper functions for tests

func filterRelsByKind(rels []Relationship, kind RelationshipKind) []Relationship {
	var filtered []Relationship
	for _, r := range rels {
		if r.Kind == kind {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func extractTargetPaths(rels []Relationship) []string {
	var paths []string
	for _, r := range rels {
		if r.TargetPath != "" {
			paths = append(paths, r.TargetPath)
		}
	}
	return paths
}
