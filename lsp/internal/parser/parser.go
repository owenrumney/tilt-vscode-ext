// Package parser turns Tiltfile text into a Starlark AST and keeps the result
// per open document.
package parser

import (
	"sync"

	"go.starlark.net/syntax"
)

// Document is one open Tiltfile: its text, its AST, and the parse error if it
// has one. File and Err are mutually exclusive.
type Document struct {
	URI     string
	Version int
	Text    string
	File    *syntax.File
	Err     *syntax.Error
}

// Cache holds the parsed form of every open document.
type Cache struct {
	mu   sync.RWMutex
	docs map[string]*Document
}

func NewCache() *Cache {
	return &Cache{docs: make(map[string]*Document)}
}

// Set parses text and stores the result, replacing any previous version.
func (c *Cache) Set(uri string, version int, text string) *Document {
	doc := Parse(uri, version, text)
	c.mu.Lock()
	c.docs[uri] = doc
	c.mu.Unlock()
	return doc
}

func (c *Cache) Get(uri string) (*Document, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	doc, ok := c.docs[uri]
	return doc, ok
}

func (c *Cache) Delete(uri string) {
	c.mu.Lock()
	delete(c.docs, uri)
	c.mu.Unlock()
}

// Parse builds a Document. A Tiltfile is Starlark with no special dialect
// flags, so the zero Mode is right.
func Parse(uri string, version int, text string) *Document {
	doc := &Document{URI: uri, Version: version, Text: text}
	file, err := syntax.Parse(uri, text, 0)
	if err != nil {
		// Parse reports the first error only, and always as *syntax.Error.
		if se, ok := err.(syntax.Error); ok {
			doc.Err = &se
			return doc
		}
		if se, ok := err.(*syntax.Error); ok {
			doc.Err = se
			return doc
		}
		doc.Err = &syntax.Error{Msg: err.Error()}
		return doc
	}
	doc.File = file
	return doc
}
