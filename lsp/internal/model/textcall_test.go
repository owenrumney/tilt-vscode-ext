package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextCallSiteOnAHalfTypedCall(t *testing.T) {
	tests := []struct {
		name     string
		before   string
		callee   string
		keyword  string
		argIndex int
	}{
		{
			name:     "just after the open paren",
			before:   "local_resource(",
			callee:   "local_resource",
			argIndex: 0,
		},
		{
			name:     "after a positional and a comma",
			before:   "local_resource('web', ",
			callee:   "local_resource",
			argIndex: 1,
		},
		{
			name:     "right after a keyword equals",
			before:   "local_resource('web', cmd='x', trigger_mode=",
			callee:   "local_resource",
			keyword:  "trigger_mode",
			argIndex: 2,
		},
		{
			name:     "a dotted callee",
			before:   "x = os.path.join(",
			callee:   "os.path.join",
			argIndex: 0,
		},
		{
			name:     "the innermost unclosed call wins",
			before:   "local_resource('web', links=[link(",
			callee:   "link",
			argIndex: 0,
		},
		{
			name:     "a closed inner call does not capture the cursor",
			before:   "local_resource('web', links=[link('a', 'b')], ",
			callee:   "local_resource",
			argIndex: 2,
		},
		{
			name:     "a comma inside a string is not a separator",
			before:   "local_resource('a,b,c', ",
			callee:   "local_resource",
			argIndex: 1,
		},
		{
			name:     "spanning lines",
			before:   "local_resource(\n    'web',\n    cmd=",
			callee:   "local_resource",
			keyword:  "cmd",
			argIndex: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			site, ok := TextCallSite(tt.before)
			require.True(t, ok)
			assert.Equal(t, tt.callee, site.Callee)
			assert.Equal(t, tt.keyword, site.Keyword)
			assert.Equal(t, tt.argIndex, site.ArgIndex)
		})
	}
}

func TestTextCallSiteCollectsSuppliedKeywords(t *testing.T) {
	site, ok := TextCallSite("local_resource('web', cmd='x', labels=['a'], ")
	require.True(t, ok)

	assert.True(t, site.Supplied["cmd"])
	assert.True(t, site.Supplied["labels"])
	assert.False(t, site.Supplied["serve_cmd"])
}

func TestTextCallSiteFindsNothingOutsideACall(t *testing.T) {
	for _, before := range []string{
		"",
		"x = 1",
		"local_resource('web', cmd='x')",
		"# local_resource(",
		"'local_resource('",
	} {
		_, ok := TextCallSite(before)
		assert.False(t, ok, "%q", before)
	}
}

func TestTextCallSiteIgnoresACommentedOpenParen(t *testing.T) {
	_, ok := TextCallSite("# see local_resource(\nx = 1")
	assert.False(t, ok)
}
