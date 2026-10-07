package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/syntax"
)

func analyse(t *testing.T, src string) *File {
	t.Helper()
	file, err := syntax.Parse("Tiltfile", src, 0)
	require.NoError(t, err)
	return Analyse(file)
}

func names(occs []Occurrence) []string {
	out := make([]string, 0, len(occs))
	for _, o := range occs {
		out = append(out, o.Name)
	}
	return out
}

func TestAnalyseNilFile(t *testing.T) {
	f := Analyse(nil)

	assert.Empty(t, f.Uses)
	assert.Empty(t, f.Resources())
	assert.Empty(t, f.Symbols())
}

func TestResourceNamesFromPositionalAndKeyword(t *testing.T) {
	f := analyse(t, `
local_resource('web', cmd='echo hi')
local_resource(name='docs', cmd='echo hi')
k8s_resource('api', port_forwards='3020:80')
k8s_resource(workload='flaky')
k8s_resource(workload='old', new_name='renamed')
dc_resource('compose-svc')
k8s_custom_deploy('custom', apply_cmd='x', delete_cmd='y', deps=[])
`)

	assert.ElementsMatch(t,
		[]string{"web", "docs", "api", "flaky", "renamed", "compose-svc", "custom"},
		names(f.Resources()),
	)
}

func TestNewNameWinsOverWorkload(t *testing.T) {
	f := analyse(t, "k8s_resource(workload='deploy', new_name='friendly')\n")

	assert.Equal(t, []string{"friendly"}, names(f.Resources()))
}

func TestResourceDepsAreReferences(t *testing.T) {
	f := analyse(t, `
local_resource('db-migrate', cmd='x')
local_resource('seed-data', cmd='y', resource_deps=['db-migrate'])
local_resource('api', cmd='z', resource_deps=['db-migrate', 'seed-data'])
`)

	// 'db-migrate' on line 2 column 16 is its declaration.
	decl, ok := f.Decls[KindResource]["db-migrate"]
	require.True(t, ok)
	assert.Equal(t, 2, decl.Span.StartLine)

	refs := f.References(decl.Span.StartLine, decl.Span.StartCol, false)
	assert.Len(t, refs, 2, "two resource_deps mention db-migrate")
	for _, r := range refs {
		assert.False(t, r.Decl)
		assert.Equal(t, "db-migrate", r.Name)
	}

	withDecl := f.References(decl.Span.StartLine, decl.Span.StartCol, true)
	assert.Len(t, withDecl, 3)
}

func TestDeclAtJumpsFromRefToDecl(t *testing.T) {
	f := analyse(t, `
local_resource('db-migrate', cmd='x')
local_resource('seed-data', cmd='y', resource_deps=['db-migrate'])
`)

	// Find the reference on line 3, then ask where it is declared.
	var ref Occurrence
	for _, u := range f.Uses {
		if u.Name == "db-migrate" && !u.Decl {
			ref = u
		}
	}
	require.Equal(t, 3, ref.Span.StartLine)

	decl, ok := f.DeclAt(ref.Span.StartLine, ref.Span.StartCol)
	require.True(t, ok)
	assert.Equal(t, 2, decl.Span.StartLine)
	assert.True(t, decl.Decl)
}

func TestSymbolsFromDefAndAssignment(t *testing.T) {
	f := analyse(t, `
public = os.path.abspath('public')

def site(name, port):
    local_resource(name, cmd='echo %s' % port)

site('web', 3010)
`)

	assert.Equal(t, []string{"public", "site"}, names(f.Symbols()))
}

func TestSymbolReferenceFindsTheDef(t *testing.T) {
	src := `
def site(name):
    local_resource(name, cmd='x')

site('web')
site('docs')
`
	f := analyse(t, src)

	decl, ok := f.Decls[KindSymbol]["site"]
	require.True(t, ok)
	assert.Equal(t, 2, decl.Span.StartLine, "the def is on line 2")

	// The call on line 5 resolves back to the def.
	target, ok := f.DeclAt(5, 1)
	require.True(t, ok)
	assert.Equal(t, 2, target.Span.StartLine)

	refs := f.References(5, 1, false)
	assert.Len(t, refs, 2, "two call sites")
}

func TestLoadDeclaresItsNames(t *testing.T) {
	f := analyse(t, `load('./ext.star', 'helper')`)

	assert.Contains(t, names(f.Symbols()), "helper")
	assert.Contains(t, names(f.Paths), "./ext.star")
}

func TestPathLiterals(t *testing.T) {
	f := analyse(t, `
k8s_yaml(['k8s/sites.yaml', 'k8s/jobs.yaml'])
read_file('config/local.yaml')
docker_build('ref', 'ctx')
local_resource('web', cmd='x', dir='subdir')
public = os.path.abspath('public')
`)

	assert.ElementsMatch(t,
		[]string{"k8s/sites.yaml", "k8s/jobs.yaml", "config/local.yaml", "ref", "ctx", "subdir", "public"},
		names(f.Paths),
	)
}

func TestPathAt(t *testing.T) {
	f := analyse(t, "k8s_yaml('k8s/sites.yaml')\n")

	require.Len(t, f.Paths, 1)
	p := f.Paths[0]
	got, ok := f.PathAt(p.Span.StartLine, p.Span.StartCol)
	require.True(t, ok)
	assert.Equal(t, "k8s/sites.yaml", got.Name)

	_, ok = f.PathAt(99, 1)
	assert.False(t, ok)
}

func TestDuplicateDeclarationKeepsTheFirst(t *testing.T) {
	f := analyse(t, `
local_resource('web', cmd='first')
local_resource('web', cmd='second')
`)

	decl := f.Decls[KindResource]["web"]
	assert.Equal(t, 2, decl.Span.StartLine)
}

func TestOccurrenceAtMissesWhitespace(t *testing.T) {
	f := analyse(t, "local_resource('web', cmd='x')\n")

	_, ok := f.OccurrenceAt(1, 200)
	assert.False(t, ok)
	assert.Nil(t, f.References(1, 200, true))
}
