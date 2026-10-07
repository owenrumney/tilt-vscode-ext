package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const good = `local_resource('web', cmd='echo hi', labels=['a'])
`

func TestParseGoodTiltfile(t *testing.T) {
	doc := Parse("file:///Tiltfile", 1, good)

	require.Nil(t, doc.Err)
	require.NotNil(t, doc.File)
	assert.Len(t, doc.File.Stmts, 1)
}

func TestParseReportsTheFirstError(t *testing.T) {
	doc := Parse("file:///Tiltfile", 1, "local_resource('web'\n")

	require.NotNil(t, doc.Err)
	assert.Nil(t, doc.File)
	assert.NotEmpty(t, doc.Err.Msg)
	// Starlark positions are 1-based, and an unclosed call is reported on the
	// line where it runs out of input.
	assert.Positive(t, doc.Err.Pos.Line)
}

func TestParseEmptyFileIsValid(t *testing.T) {
	doc := Parse("file:///Tiltfile", 1, "")

	require.Nil(t, doc.Err)
	require.NotNil(t, doc.File)
	assert.Empty(t, doc.File.Stmts)
}

func TestParseKeepsRealTiltfileConstructs(t *testing.T) {
	// A def, a comprehension, a format operator and a multi-line call: the
	// shapes demo/Tiltfile actually uses.
	src := `
public = os.path.abspath('public')

def site(name, port, urls):
    local_resource(
        name,
        cmd='printf "building %s" % name',
        links=[link(url, title) for (url, title) in urls],
        allow_parallel=True,
    )

site('web', 3010, [('http://localhost:3010', 'web')])
`
	doc := Parse("file:///Tiltfile", 1, src)

	require.Nil(t, doc.Err, "msg: %v", doc.Err)
	assert.Len(t, doc.File.Stmts, 3)
}

func TestCacheLifecycle(t *testing.T) {
	c := NewCache()
	uri := "file:///Tiltfile"

	_, ok := c.Get(uri)
	assert.False(t, ok)

	c.Set(uri, 1, good)
	doc, ok := c.Get(uri)
	require.True(t, ok)
	assert.Equal(t, 1, doc.Version)

	c.Set(uri, 2, "x = 1\n")
	doc, _ = c.Get(uri)
	assert.Equal(t, 2, doc.Version, "a later version replaces the earlier one")

	c.Delete(uri)
	_, ok = c.Get(uri)
	assert.False(t, ok)
}
