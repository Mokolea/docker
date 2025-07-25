package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/network"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func TestNetworkInspect(t *testing.T) {
	client := &Client{
		client: newMockClient(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				return nil, errors.New("expected GET method, got " + req.Method)
			}
			if req.URL.Path == "/networks/" {
				return errorMock(http.StatusInternalServerError, "client should not make a request for empty IDs")(req)
			}
			if strings.HasPrefix(req.URL.Path, "/networks/unknown") {
				return errorMock(http.StatusNotFound, "Error: No such network: unknown")(req)
			}
			if strings.HasPrefix(req.URL.Path, "/networks/test-500-response") {
				return errorMock(http.StatusInternalServerError, "Server error")(req)
			}
			// other test-cases all use "network_id"
			if !strings.HasPrefix(req.URL.Path, "/networks/network_id") {
				return nil, errors.New("expected URL '/networks/network_id', got " + req.URL.Path)
			}
			if strings.Contains(req.URL.RawQuery, "scope=global") {
				return errorMock(http.StatusNotFound, "Error: No such network: network_id")(req)
			}
			var (
				content []byte
				err     error
			)
			if strings.Contains(req.URL.RawQuery, "verbose=true") {
				s := map[string]network.ServiceInfo{
					"web": {},
				}
				content, err = json.Marshal(network.Inspect{
					Name:     "mynetwork",
					Services: s,
				})
			} else {
				content, err = json.Marshal(network.Inspect{
					Name: "mynetwork",
				})
			}
			if err != nil {
				return nil, err
			}
			return &http.Response{
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(content)),
			}, nil
		}),
	}

	t.Run("empty ID", func(t *testing.T) {
		// verify that the client does not create a request if the network-ID/name is empty.
		_, err := client.NetworkInspect(context.Background(), "", network.InspectOptions{})
		assert.Check(t, is.ErrorType(err, cerrdefs.IsInvalidArgument))
		assert.Check(t, is.ErrorContains(err, "value is empty"))

		_, err = client.NetworkInspect(context.Background(), "    ", network.InspectOptions{})
		assert.Check(t, is.ErrorType(err, cerrdefs.IsInvalidArgument))
		assert.Check(t, is.ErrorContains(err, "value is empty"))
	})
	t.Run("no options", func(t *testing.T) {
		r, err := client.NetworkInspect(context.Background(), "network_id", network.InspectOptions{})
		assert.NilError(t, err)
		assert.Check(t, is.Equal(r.Name, "mynetwork"))
	})
	t.Run("verbose", func(t *testing.T) {
		r, err := client.NetworkInspect(context.Background(), "network_id", network.InspectOptions{Verbose: true})
		assert.NilError(t, err)
		assert.Check(t, is.Equal(r.Name, "mynetwork"))
		_, ok := r.Services["web"]
		assert.Check(t, ok, "expected service `web` missing in the verbose output")
	})
	t.Run("global scope", func(t *testing.T) {
		_, err := client.NetworkInspect(context.Background(), "network_id", network.InspectOptions{Scope: "global"})
		assert.Check(t, is.ErrorContains(err, "Error: No such network: network_id"))
		assert.Check(t, is.ErrorType(err, cerrdefs.IsNotFound))
	})
	t.Run("unknown network", func(t *testing.T) {
		_, err := client.NetworkInspect(context.Background(), "unknown", network.InspectOptions{})
		assert.Check(t, is.ErrorContains(err, "Error: No such network: unknown"))
		assert.Check(t, is.ErrorType(err, cerrdefs.IsNotFound))
	})
	t.Run("server error", func(t *testing.T) {
		// Just testing that an internal server error is converted correctly by the client
		_, err := client.NetworkInspect(context.Background(), "test-500-response", network.InspectOptions{})
		assert.Check(t, is.ErrorType(err, cerrdefs.IsInternal))
	})
}
