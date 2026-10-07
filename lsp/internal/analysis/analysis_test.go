package analysis

import (
	"testing"

	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/syntax"
)

func run(t *testing.T, src string) []Finding {
	t.Helper()
	file, err := syntax.Parse("Tiltfile", src, 0)
	require.NoError(t, err, "fixture must parse")
	return Run(file, model.Analyse(file))
}

func codes(findings []Finding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.Code)
	}
	return out
}

func only(t *testing.T, findings []Finding, code string) Finding {
	t.Helper()
	var hits []Finding
	for _, f := range findings {
		if f.Code == code {
			hits = append(hits, f)
		}
	}
	require.Len(t, hits, 1, "expected exactly one %s, got %v", code, codes(findings))
	return hits[0]
}

func TestNilInputs(t *testing.T) {
	assert.Nil(t, Run(nil, nil))
}

func TestCleanTiltfileIsSilent(t *testing.T) {
	findings := run(t, `
local_resource('db-migrate', cmd='x')
local_resource('seed-data', cmd='y', resource_deps=['db-migrate'])
k8s_resource('web', port_forwards='3010:80')
local_resource('nightly', cmd='z', trigger_mode=TRIGGER_MODE_MANUAL, auto_init=False)
`)
	assert.Empty(t, codes(findings))
}

func TestDuplicateResource(t *testing.T) {
	findings := run(t, `
local_resource('web', cmd='first')
local_resource('web', cmd='second')
`)
	f := only(t, findings, "duplicate-resource")
	assert.Equal(t, Error, f.Severity)
	assert.Contains(t, f.Message, `"web" is already declared on line 2`)
	assert.Equal(t, 3, f.Span.StartLine, "the second declaration is flagged")
}

func TestUnknownResourceDep(t *testing.T) {
	findings := run(t, `
local_resource('web', cmd='x')
local_resource('api', cmd='y', resource_deps=['wen'])
`)
	f := only(t, findings, "unknown-resource-dep")
	assert.Equal(t, Error, f.Severity)
	assert.Contains(t, f.Message, `no resource named "wen"`)
	require.NotNil(t, f.Fix, "a one-letter typo gets a fix")
	assert.Equal(t, `'web'`, f.Fix.NewText)
}

func TestUnknownResourceDepWithNoNearMatchHasNoFix(t *testing.T) {
	findings := run(t, `
local_resource('web', cmd='x')
local_resource('api', cmd='y', resource_deps=['something-else-entirely'])
`)
	f := only(t, findings, "unknown-resource-dep")
	assert.Nil(t, f.Fix)
}

// The corpus showed that most Tiltfiles with a helper build names from a
// variable, so absence cannot be proven.
func TestUnknownResourceDepSuppressedByANonLiteralName(t *testing.T) {
	findings := run(t, `
def site(name):
    local_resource(name, cmd='x')

site('web')
local_resource('api', cmd='y', resource_deps=['web'])
`)
	assert.NotContains(t, codes(findings), "unknown-resource-dep")
}

func TestUnknownResourceDepSuppressedByAnExtensionLoad(t *testing.T) {
	findings := run(t, `
load('ext://restart_process', 'docker_build_with_restart')
local_resource('api', cmd='y', resource_deps=['from-the-extension'])
`)
	assert.NotContains(t, codes(findings), "unknown-resource-dep")
}

// A loaded file is Starlark that may call local_resource itself — a helper
// .star, or another Tiltfile pulled in for its resources. This server does not
// parse it, so a local load is exactly as opaque as a remote one.
func TestAnyLoadOpensTheResourceSet(t *testing.T) {
	loads := []string{
		"load('./helpers.star', 'helper')",
		"load('./other/Tiltfile', 'helper')",
		"load('ext://restart_process', 'docker_build_with_restart')",
		"include('./other/Tiltfile')",
	}
	for _, loader := range loads {
		t.Run(loader, func(t *testing.T) {
			findings := run(t, loader+`
local_resource('web', cmd='x')
local_resource('api', cmd='y', resource_deps=['declared-elsewhere'])
`)
			assert.NotContains(t, codes(findings), "unknown-resource-dep")
			assert.NotContains(t, codes(findings), "resource-dep-cycle")
		})
	}
}

// A local load still leaves the builtin check running: the names it binds are
// written in the load statement, so they are visible. Only an ext:// module
// can export more than it names.
func TestLocalLoadKeepsTheBuiltinCheck(t *testing.T) {
	findings := run(t, `
load('./helpers.star', 'helper')
local_resourse('web', cmd='x')
`)
	assert.Contains(t, codes(findings), "unknown-builtin")
}

func TestDependencyCycle(t *testing.T) {
	findings := run(t, `
local_resource('a', cmd='x', resource_deps=['b'])
local_resource('b', cmd='y', resource_deps=['c'])
local_resource('c', cmd='z', resource_deps=['a'])
`)
	f := only(t, findings, "resource-dep-cycle")
	assert.Equal(t, Error, f.Severity)
	assert.Equal(t, "resource_deps cycle: a -> b -> c -> a", f.Message)
}

func TestSelfDependencyIsACycle(t *testing.T) {
	findings := run(t, `local_resource('a', cmd='x', resource_deps=['a'])`)

	f := only(t, findings, "resource-dep-cycle")
	assert.Equal(t, "resource_deps cycle: a -> a", f.Message)
}

func TestDiamondIsNotACycle(t *testing.T) {
	findings := run(t, `
local_resource('base', cmd='x')
local_resource('left', cmd='x', resource_deps=['base'])
local_resource('right', cmd='x', resource_deps=['base'])
local_resource('top', cmd='x', resource_deps=['left', 'right'])
`)
	assert.NotContains(t, codes(findings), "resource-dep-cycle")
}

func TestPortForwards(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{"8080", true},
		{"9000:8080", true},
		{"localhost:9000:8080", true},
		{"3110:80", true},
		{"65535", true},
		{"", false},
		{"80:", false},
		{":80", false},
		{"http", false},
		{"8080:80:1:2", false},
		{"0", false},
		{"70000", false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			assert.Equal(t, tt.valid, validPortForward(tt.value))
		})
	}
}

func TestBadPortForwardIsReported(t *testing.T) {
	findings := run(t, `k8s_resource('web', port_forwards='http')`)

	f := only(t, findings, "bad-port-forward")
	assert.Equal(t, Error, f.Severity)
	assert.Contains(t, f.Message, "local_port[:container_port] or host:local_port:container_port")
}

func TestBuiltPortForwardIsLeftAlone(t *testing.T) {
	findings := run(t, `
port = '3010'
k8s_resource('web', port_forwards='%s:80' % port)
`)
	assert.NotContains(t, codes(findings), "bad-port-forward")
}

func TestUnknownBuiltin(t *testing.T) {
	findings := run(t, `local_resourse('web', cmd='x')`)

	f := only(t, findings, "unknown-builtin")
	assert.Equal(t, Warning, f.Severity, "never an error: see the corpus findings")
	require.NotNil(t, f.Fix)
	assert.Equal(t, "local_resource", f.Fix.NewText)
}

func TestUnknownBuiltinSuppressedByAnExtensionLoad(t *testing.T) {
	findings := run(t, `
load('ext://restart_process', 'docker_build_with_restart')
docker_build_with_restart('ref', '.')
kind_load('ref')
`)
	assert.NotContains(t, codes(findings), "unknown-builtin")
}

func TestLocalFunctionsAreNotUnknown(t *testing.T) {
	findings := run(t, `
def site(name):
    local_resource(name, cmd='x')

site('web')
`)
	assert.NotContains(t, codes(findings), "unknown-builtin")
}

func TestStarlarkBuiltinsAreNotUnknown(t *testing.T) {
	findings := run(t, `
names = sorted(set(['a', 'b']))
count = len(names)
for i, n in enumerate(names):
    print(str(i) + n)
`)
	assert.NotContains(t, codes(findings), "unknown-builtin")
}

func TestMethodCallsAreNotUnknown(t *testing.T) {
	findings := run(t, `
home = os.environ.get('HOME')
parts = 'a,b'.split(',')
`)
	assert.NotContains(t, codes(findings), "unknown-builtin")
}

func TestLoopVariableIsNotUnknown(t *testing.T) {
	findings := run(t, `
for svc in ['a', 'b']:
    local_resource(svc, cmd='x')
`)
	assert.NotContains(t, codes(findings), "unknown-builtin")
}

func TestOtherBindingFormsAreNotUnknown(t *testing.T) {
	findings := run(t, `
def pick():
    return str, repr

fn, other = pick()
fn('x')
names = [f('y') for f in pick()]
for name, build in {'a': str}.items():
    build(name)
`)
	assert.NotContains(t, codes(findings), "unknown-builtin")
}

func TestIncludeOpensTheResourceSet(t *testing.T) {
	findings := run(t, `
include('./backend/Tiltfile')
local_resource('api', cmd='x', resource_deps=['db'])
`)
	assert.NotContains(t, codes(findings), "unknown-resource-dep")
}

func TestFindingsAreSortedByPosition(t *testing.T) {
	findings := run(t, `
k8s_resource('web', port_forwards='nope')
local_resource('a', cmd='x')
local_resource('a', cmd='y')
`)
	require.Len(t, findings, 2)
	assert.Less(t, findings[0].Span.StartLine, findings[1].Span.StartLine)
}

func TestNearestKeepsItsDistance(t *testing.T) {
	// A short name gets a tighter limit, or every typo matches something.
	assert.Equal(t, "", nearest("ab", []string{"xyz"}))
	assert.Equal(t, "web", nearest("wen", []string{"web", "api"}))
	assert.Equal(t, "", nearest("totally-different", []string{"web", "api"}))
}

// auto_init=False is legitimate. It stops the resource running at tilt up;
// with an automatic trigger mode the resource still runs when its deps
// change. Reporting it produced 24 false findings across the corpus.
func TestAutoInitIsNotReported(t *testing.T) {
	findings := run(t, `
local_resource('nightly', cmd='x', auto_init=False)
local_resource('watched', cmd='y', deps=['src'], auto_init=False)
local_resource('manual', cmd='z', auto_init=False, trigger_mode=TRIGGER_MODE_MANUAL)
`)
	assert.Empty(t, codes(findings))
}

// A second k8s_resource for an existing name configures it rather than
// redeclaring it, which is how objects are attached to a renamed workload.
// Treating it as a duplicate produced 14 false findings in one real Tiltfile.
func TestSecondK8sResourceForTheSameNameIsNotADuplicate(t *testing.T) {
	findings := run(t, `
k8s_resource(workload='brigade-apiserver', new_name='apiserver')
k8s_resource(workload='apiserver', objects=['brigade-apiserver:clusterrole'])
`)
	assert.NotContains(t, codes(findings), "duplicate-resource")
}

func TestDuplicateLocalResourceIsStillReported(t *testing.T) {
	findings := run(t, `
local_resource('web', cmd='first')
local_resource('web', cmd='second')
`)
	f := only(t, findings, "duplicate-resource")
	assert.Contains(t, f.Message, `local_resource "web" is already declared on line 2`)
}

// A manifest loader creates resources named after workloads in a YAML, or
// services in a compose file, which this server never reads.
func TestManifestLoaderSuppressesTheMissingResourceCheck(t *testing.T) {
	loaders := []string{
		"k8s_yaml('deploy.yaml')",
		"docker_compose('compose.yaml')",
		"helm('./chart')",
		"kustomize('./overlay')",
	}
	for _, loader := range loaders {
		t.Run(loader, func(t *testing.T) {
			findings := run(t, loader+"\nlocal_resource('api', cmd='y', resource_deps=['postgres'])\n")
			assert.NotContains(t, codes(findings), "unknown-resource-dep")
			assert.NotContains(t, codes(findings), "resource-dep-cycle")
		})
	}
}

func TestNestedDefIsNotAnUnknownBuiltin(t *testing.T) {
	findings := run(t, `
def outer(version):
    def deploy(v):
        return 'kubectl apply ' + v

    def wait():
        return 'kubectl wait'

    local_resource('deploy-it', deploy(version))
    local_resource('wait-for-it', wait())
`)
	assert.NotContains(t, codes(findings), "unknown-builtin")
}

// Two branches are alternatives, not repetitions. One real Tiltfile declares
// the same local_resource in both arms of an if, which is correct code.
func TestSameResourceInBothBranchesIsNotADuplicate(t *testing.T) {
	findings := run(t, `
ENABLE_WEBHOOKS = True

if ENABLE_WEBHOOKS:
    local_resource('apply-sample', cmd='retry-until-ready')
else:
    local_resource('apply-sample', cmd='kubectl apply')
`)
	assert.NotContains(t, codes(findings), "duplicate-resource")
}

func TestConditionalResourceSuppressesTheMissingResourceCheck(t *testing.T) {
	findings := run(t, `
if True:
    local_resource('maybe', cmd='x')

local_resource('api', cmd='y', resource_deps=['maybe'])
`)
	assert.NotContains(t, codes(findings), "unknown-resource-dep")
}

// A builtin Tilt has removed is the one report the corpus confirms as real.
func TestRemovedBuiltinIsReported(t *testing.T) {
	findings := run(t, `repo = local_git_repo('.')`)

	f := only(t, findings, "unknown-builtin")
	assert.Equal(t, Warning, f.Severity)
	assert.Contains(t, f.Message, "local_git_repo")
}
