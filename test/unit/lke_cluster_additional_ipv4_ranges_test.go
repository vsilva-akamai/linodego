package unit

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/linode/linodego/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// additionalIPv4RangesKey is the JSON key of the requested additional IPv4 range list.
const additionalIPv4RangesKey = "additional_ipv4_ranges"

// marshalToMap marshals v and decodes the result into a generic map so that tests can assert
// on the presence, absence, and exact shape of individual JSON keys.
func marshalToMap(t *testing.T, v any) map[string]any {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	return parsed
}

func TestLKECluster_AdditionalIPv4Ranges_CreateSerialization(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ranges   []linodego.LKEClusterAdditionalIPv4Range
		expected any
		present  bool
	}{
		{
			name:    "omitted when not requested",
			ranges:  nil,
			present: false,
		},
		{
			name:    "single range",
			ranges:  []linodego.LKEClusterAdditionalIPv4Range{{Range: "7.0.0.0/8"}},
			present: true,
			expected: []any{
				map[string]any{"range": "7.0.0.0/8"},
			},
		},
		{
			name: "multiple ranges keep the order they were supplied in",
			ranges: []linodego.LKEClusterAdditionalIPv4Range{
				{Range: "9.0.0.0/8"},
				{Range: "7.0.0.0/8"},
				{Range: "11.0.0.0/8"},
			},
			present: true,
			expected: []any{
				map[string]any{"range": "9.0.0.0/8"},
				map[string]any{"range": "7.0.0.0/8"},
				map[string]any{"range": "11.0.0.0/8"},
			},
		},
		{
			// An empty list is not a way to say "use the defaults" and the API rejects it, so
			// the SDK must send it as-is rather than quietly dropping it and changing the
			// caller's request into a valid one.
			name:     "explicitly empty list is sent",
			ranges:   []linodego.LKEClusterAdditionalIPv4Range{},
			present:  true,
			expected: []any{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := linodego.LKEClusterCreateOptions{
				Label:                "test-cluster",
				Region:               "us-lax",
				Tier:                 "enterprise",
				AdditionalIPv4Ranges: tc.ranges,
			}

			parsed := marshalToMap(t, opts)

			value, ok := parsed[additionalIPv4RangesKey]
			if !tc.present {
				assert.False(t, ok, "%s should be omitted when no ranges are requested", additionalIPv4RangesKey)
				return
			}

			require.True(t, ok, "%s should be present", additionalIPv4RangesKey)
			assert.Equal(t, tc.expected, value)
		})
	}
}

// TestLKECluster_AdditionalIPv4Ranges_CreateRequestUnchanged proves that a caller which does not
// use the new field serializes exactly the same request as before it existed.
func TestLKECluster_AdditionalIPv4Ranges_CreateRequestUnchanged(t *testing.T) {
	opts := linodego.LKEClusterCreateOptions{
		Label:      "test-cluster",
		Region:     "us-west",
		K8sVersion: "1.22",
		NodePools: []linodego.LKENodePoolCreateOptions{
			{Count: 1, Type: "g6-standard-2"},
		},
	}

	data, err := json.Marshal(opts)
	require.NoError(t, err)

	assert.NotContains(t, string(data), additionalIPv4RangesKey)
}

func TestLKECluster_AdditionalIPv4Ranges_Create(t *testing.T) {
	fixtureData, err := fixtures.GetFixture("lke_cluster_additional_ipv4_ranges_create")
	assert.NoError(t, err)

	var base ClientBaseCase
	base.SetUp(t)
	defer base.TearDown(t)

	requested := []linodego.LKEClusterAdditionalIPv4Range{
		{Range: "7.0.0.0/8"},
		{Range: "9.0.0.0/8"},
	}

	base.MockPost("lke/clusters", fixtureData)

	cluster, err := base.Client.CreateLKECluster(context.Background(), linodego.LKEClusterCreateOptions{
		Label:                "new-enterprise-cluster",
		Region:               "us-lax",
		Tier:                 "enterprise",
		AdditionalIPv4Ranges: requested,
	})
	require.NoError(t, err)

	assert.Equal(t, "new-enterprise-cluster", cluster.Label)
	assert.Equal(t, requested, cluster.AdditionalIPv4Ranges)
}

// TestLKECluster_AdditionalIPv4Ranges_CallerSliceIsolation proves the SDK neither reorders nor
// otherwise mutates the slice the caller owns, and that the decoded response is backed by its
// own storage rather than by the caller's slice.
func TestLKECluster_AdditionalIPv4Ranges_CallerSliceIsolation(t *testing.T) {
	fixtureData, err := fixtures.GetFixture("lke_cluster_additional_ipv4_ranges_create")
	assert.NoError(t, err)

	var base ClientBaseCase
	base.SetUp(t)
	defer base.TearDown(t)

	requested := []linodego.LKEClusterAdditionalIPv4Range{
		{Range: "9.0.0.0/8"},
		{Range: "7.0.0.0/8"},
	}
	original := append([]linodego.LKEClusterAdditionalIPv4Range(nil), requested...)

	base.MockPost("lke/clusters", fixtureData)

	cluster, err := base.Client.CreateLKECluster(context.Background(), linodego.LKEClusterCreateOptions{
		Label:                "new-enterprise-cluster",
		Region:               "us-lax",
		Tier:                 "enterprise",
		AdditionalIPv4Ranges: requested,
	})
	require.NoError(t, err)

	assert.Equal(t, original, requested, "the caller's slice must not be modified")

	// Mutating the caller's slice afterwards must not reach through into the response.
	beforeMutation := append([]linodego.LKEClusterAdditionalIPv4Range(nil), cluster.AdditionalIPv4Ranges...)
	requested[0].Range = "10.0.0.0/8"
	assert.Equal(t, beforeMutation, cluster.AdditionalIPv4Ranges)
}

func TestLKECluster_AdditionalIPv4Ranges_Get(t *testing.T) {
	fixtureData, err := fixtures.GetFixture("lke_cluster_additional_ipv4_ranges_get")
	assert.NoError(t, err)

	var base ClientBaseCase
	base.SetUp(t)
	defer base.TearDown(t)

	base.MockGet("lke/clusters/126", fixtureData)

	cluster, err := base.Client.GetLKECluster(context.Background(), 126)
	require.NoError(t, err)

	assert.Equal(t, "enterprise", cluster.Tier)
	assert.Equal(t, []linodego.LKEClusterAdditionalIPv4Range{
		{Range: "7.0.0.0/8"},
		{Range: "9.0.0.0/8"},
	}, cluster.AdditionalIPv4Ranges)
}

func TestLKECluster_AdditionalIPv4Ranges_GetNull(t *testing.T) {
	fixtureData, err := fixtures.GetFixture("lke_cluster_additional_ipv4_ranges_null_get")
	assert.NoError(t, err)

	var base ClientBaseCase
	base.SetUp(t)
	defer base.TearDown(t)

	base.MockGet("lke/clusters/127", fixtureData)

	cluster, err := base.Client.GetLKECluster(context.Background(), 127)
	require.NoError(t, err)

	assert.Nil(t, cluster.AdditionalIPv4Ranges)
}

// TestLKECluster_AdditionalIPv4Ranges_GetAbsent covers a cluster payload that predates the field
// entirely, such as one returned by an API version that does not serve it.
func TestLKECluster_AdditionalIPv4Ranges_GetAbsent(t *testing.T) {
	fixtureData, err := fixtures.GetFixture("lke_cluster_get")
	assert.NoError(t, err)

	var base ClientBaseCase
	base.SetUp(t)
	defer base.TearDown(t)

	base.MockGet("lke/clusters/123", fixtureData)

	cluster, err := base.Client.GetLKECluster(context.Background(), 123)
	require.NoError(t, err)

	assert.Nil(t, cluster.AdditionalIPv4Ranges)
}

func TestLKECluster_AdditionalIPv4Ranges_List(t *testing.T) {
	fixtureData, err := fixtures.GetFixture("lke_cluster_additional_ipv4_ranges_list")
	assert.NoError(t, err)

	var base ClientBaseCase
	base.SetUp(t)
	defer base.TearDown(t)

	base.MockGet("lke/clusters", fixtureData)

	clusters, err := base.Client.ListLKEClusters(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, clusters, 3)

	assert.Equal(t, []linodego.LKEClusterAdditionalIPv4Range{
		{Range: "7.0.0.0/8"},
		{Range: "9.0.0.0/8"},
	}, clusters[0].AdditionalIPv4Ranges)
	assert.Nil(t, clusters[1].AdditionalIPv4Ranges, "an explicit null must decode to no ranges")
	assert.Nil(t, clusters[2].AdditionalIPv4Ranges, "an absent value must decode to no ranges")
}

// TestLKECluster_AdditionalIPv4Ranges_AbsentFromUpdate proves the field cannot be sent on an
// update, both by type and by what an update request actually serializes to.
func TestLKECluster_AdditionalIPv4Ranges_AbsentFromUpdate(t *testing.T) {
	updateType := reflect.TypeOf(linodego.LKEClusterUpdateOptions{})
	for i := range updateType.NumField() {
		tag := updateType.Field(i).Tag.Get("json")
		key, _, _ := strings.Cut(tag, ",")
		assert.NotEqual(
			t,
			additionalIPv4RangesKey,
			key,
			"LKEClusterUpdateOptions must not expose %s: the requested ranges are fixed at creation",
		)
	}

	cluster := linodego.LKECluster{
		Label: "test-cluster",
		AdditionalIPv4Ranges: []linodego.LKEClusterAdditionalIPv4Range{
			{Range: "7.0.0.0/8"},
		},
	}

	parsed := marshalToMap(t, cluster.GetUpdateOptions())
	_, ok := parsed[additionalIPv4RangesKey]
	assert.False(t, ok, "%s must never be part of an update request", additionalIPv4RangesKey)
}

func TestLKECluster_AdditionalIPv4Ranges_UpdateRequestUnchanged(t *testing.T) {
	fixtureData, err := fixtures.GetFixture("lke_cluster_update")
	assert.NoError(t, err)

	var base ClientBaseCase
	base.SetUp(t)
	defer base.TearDown(t)

	updateOptions := linodego.LKEClusterUpdateOptions{
		Label: "updated-cluster",
	}

	httpmock.RegisterRegexpResponder("PUT", mockRequestURL(t, "lke/clusters/123"),
		mockRequestBodyValidate(t, updateOptions, fixtureData))

	_, err = base.Client.UpdateLKECluster(context.Background(), 123, updateOptions)
	require.NoError(t, err)
}

// TestLKECluster_AdditionalIPv4Ranges_GetCreateOptions proves the requested ranges survive a
// round trip back into create options, in an independently owned slice.
func TestLKECluster_AdditionalIPv4Ranges_GetCreateOptions(t *testing.T) {
	cluster := linodego.LKECluster{
		Label:  "test-cluster",
		Region: "us-lax",
		AdditionalIPv4Ranges: []linodego.LKEClusterAdditionalIPv4Range{
			{Range: "7.0.0.0/8"},
			{Range: "9.0.0.0/8"},
		},
	}

	opts := cluster.GetCreateOptions()
	assert.Equal(t, cluster.AdditionalIPv4Ranges, opts.AdditionalIPv4Ranges)

	// The returned options must not alias the cluster's slice in either direction.
	opts.AdditionalIPv4Ranges[0].Range = "11.0.0.0/8"
	assert.Equal(t, "7.0.0.0/8", cluster.AdditionalIPv4Ranges[0].Range)

	cluster.AdditionalIPv4Ranges[1].Range = "12.0.0.0/8"
	assert.Equal(t, "9.0.0.0/8", opts.AdditionalIPv4Ranges[1].Range)

	// A cluster with no requested ranges produces create options with none.
	assert.Nil(t, linodego.LKECluster{Label: "test-cluster"}.GetCreateOptions().AdditionalIPv4Ranges)
}
