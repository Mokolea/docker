package system

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/docker/docker/testutil"
	"github.com/docker/docker/testutil/daemon"
	"github.com/docker/docker/testutil/request"
	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/api/types/swarm"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/skip"
)

func TestPingCacheHeaders(t *testing.T) {
	ctx := setupTest(t)

	res, _, err := request.Get(ctx, "/_ping")
	assert.NilError(t, err)
	assert.Equal(t, res.StatusCode, http.StatusOK)

	assert.Equal(t, hdr(res, "Cache-Control"), "no-cache, no-store, must-revalidate")
	assert.Equal(t, hdr(res, "Pragma"), "no-cache")
}

func TestPingGet(t *testing.T) {
	ctx := setupTest(t)

	res, body, err := request.Get(ctx, "/_ping")
	assert.NilError(t, err)

	b, err := request.ReadBody(body)
	assert.NilError(t, err)
	assert.Equal(t, string(b), "OK")
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Check(t, hdr(res, "Api-Version") != "")
}

func TestPingHead(t *testing.T) {
	ctx := setupTest(t)

	res, body, err := request.Head(ctx, "/_ping")
	assert.NilError(t, err)

	b, err := request.ReadBody(body)
	assert.NilError(t, err)
	assert.Equal(t, 0, len(b))
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Check(t, hdr(res, "Api-Version") != "")
}

func TestPingSwarmHeader(t *testing.T) {
	skip.If(t, testEnv.IsRemoteDaemon)
	skip.If(t, testEnv.DaemonInfo.OSType == "windows")

	ctx := setupTest(t)
	d := daemon.New(t)
	d.StartNode(t)
	defer d.Stop(t)
	apiClient := d.NewClientT(t)
	defer apiClient.Close()

	t.Run("before swarm init", func(t *testing.T) {
		ctx := testutil.StartSpan(ctx, t)
		p, err := apiClient.Ping(ctx)
		assert.NilError(t, err)
		assert.Equal(t, p.SwarmStatus.NodeState, swarm.LocalNodeStateInactive)
		assert.Equal(t, p.SwarmStatus.ControlAvailable, false)
	})

	_, err := apiClient.SwarmInit(ctx, swarm.InitRequest{ListenAddr: "127.0.0.1", AdvertiseAddr: "127.0.0.1:2377"})
	assert.NilError(t, err)

	t.Run("after swarm init", func(t *testing.T) {
		ctx := testutil.StartSpan(ctx, t)
		p, err := apiClient.Ping(ctx)
		assert.NilError(t, err)
		assert.Equal(t, p.SwarmStatus.NodeState, swarm.LocalNodeStateActive)
		assert.Equal(t, p.SwarmStatus.ControlAvailable, true)
	})

	err = apiClient.SwarmLeave(ctx, true)
	assert.NilError(t, err)

	t.Run("after swarm leave", func(t *testing.T) {
		ctx := testutil.StartSpan(ctx, t)
		p, err := apiClient.Ping(ctx)
		assert.NilError(t, err)
		assert.Equal(t, p.SwarmStatus.NodeState, swarm.LocalNodeStateInactive)
		assert.Equal(t, p.SwarmStatus.ControlAvailable, false)
	})
}

func TestPingBuilderHeader(t *testing.T) {
	skip.If(t, testEnv.IsRemoteDaemon)
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "cannot spin up additional daemons on windows")

	ctx := setupTest(t)
	d := daemon.New(t)
	apiClient := d.NewClientT(t)
	defer apiClient.Close()

	t.Run("default config", func(t *testing.T) {
		testutil.StartSpan(ctx, t)
		d.Start(t)
		defer d.Stop(t)

		expected := build.BuilderBuildKit
		if runtime.GOOS == "windows" {
			expected = build.BuilderV1
		}

		p, err := apiClient.Ping(ctx)
		assert.NilError(t, err)
		assert.Equal(t, p.BuilderVersion, expected)
	})

	t.Run("buildkit disabled", func(t *testing.T) {
		testutil.StartSpan(ctx, t)
		cfg := filepath.Join(d.RootDir(), "daemon.json")
		err := os.WriteFile(cfg, []byte(`{"features": { "buildkit": false }}`), 0o644)
		assert.NilError(t, err)
		d.Start(t, "--config-file", cfg)
		defer d.Stop(t)

		expected := build.BuilderV1
		p, err := apiClient.Ping(ctx)
		assert.NilError(t, err)
		assert.Equal(t, p.BuilderVersion, expected)
	})
}

func hdr(res *http.Response, name string) string {
	val, ok := res.Header[http.CanonicalHeaderKey(name)]
	if !ok || len(val) == 0 {
		return ""
	}
	return strings.Join(val, ", ")
}
