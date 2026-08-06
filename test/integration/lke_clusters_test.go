package integration

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	k8scondition "github.com/linode/linodego/k8s/pkg/condition"
	"github.com/linode/linodego/v2"
	"github.com/stretchr/testify/require"
)

func TestLKECluster_GetMissing(t *testing.T) {
	client, teardown := createTestClient(t, "fixtures/TestLKECluster_GetMissing")
	defer teardown()

	i, err := client.GetLKECluster(context.Background(), 0)
	if err == nil {
		t.Errorf("should have received an error requesting a missing lkeCluster, got %v", i)
	}
	e, ok := err.(*linodego.Error)
	if !ok {
		t.Errorf("should have received an Error requesting a missing lkeCluster, got %v", e)
	}

	if e.Code != 404 {
		t.Errorf("should have received a 404 Code requesting a missing lkeCluster, got %v", e.Code)
	}
}

func TestLKECluster_WaitForReady(t *testing.T) {
	ctx := waitContext(t, 10*60*time.Second)

	client, cluster, teardown, err := setupLKECluster(t, []clusterModifier{func(createOpts *linodego.LKEClusterCreateOptions) {
		createOpts.Label = "go-lke-test-wait"
		createOpts.NodePools = []linodego.LKENodePoolCreateOptions{
			{Count: 3, Type: "g6-standard-1"},
		}
	}}, "fixtures/TestLKECluster_WaitForReady")
	defer teardown()

	wrapper, teardownClusterClient := transportRecorderWrapper(t, "fixtures/TestLKECluster_WaitForReady_Cluster")
	defer teardownClusterClient()

	if err = k8scondition.WaitForLKEClusterReady(ctx, *client, cluster.ID, linodego.LKEClusterPollOptions{
		Retry:            true,
		TransportWrapper: wrapper,
	}); err != nil {
		t.Errorf("Error waiting for the LKE cluster pools to be ready: %s", err)
	}
}

func TestLKECluster_GetFound_smoke(t *testing.T) {
	client, lkeCluster, teardown, err := setupLKECluster(t, []clusterModifier{func(createOpts *linodego.LKEClusterCreateOptions) {
		createOpts.Label = "go-lke-test-found"
	}}, "fixtures/TestLKECluster_GetFound")
	defer teardown()
	i, err := client.GetLKECluster(context.Background(), lkeCluster.ID)
	if err != nil {
		t.Errorf("Error getting lkeCluster, expected struct, got %v and error %v", i, err)
	}
	if i.ID != lkeCluster.ID {
		t.Errorf("Expected a specific lkeCluster, but got a different one %v", i)
	}
}

func TestLKECluster_Enterprise_smoke(t *testing.T) {
	client, lkeCluster, teardown, err := setupLKECluster(t, []clusterModifier{func(createOpts *linodego.LKEClusterCreateOptions) {
		createOpts.Tier = "enterprise"
		createOpts.Region = "us-lax"
		createOpts.K8sVersion = ""
	}}, "fixtures/TestLKECluster_Enterprise_smoke")
	defer teardown()
	i, err := client.GetLKECluster(context.Background(), lkeCluster.ID)
	if err != nil {
		t.Errorf("Error getting lkeCluster, expected struct, got %v and error %v", i, err)
	}
	if i.ID != lkeCluster.ID {
		t.Errorf("Expected a specific lkeCluster, but got a different one %v", i)
	}
	if i.Tier != "enterprise" {
		t.Errorf("Expected a lkeCluster to have enterprise tier")
	}
}

// TestLKECluster_Enterprise_AdditionalIPv4Ranges_smoke creates an LKE Enterprise cluster with an
// LKE-managed VPC and an additional IPv4 range, then reads it back to confirm the request is
// returned as submitted. The cluster response carries only the requested additions: the platform's
// own default VPC ranges are not part of this field.
//
// This requires an account and region entitled to custom VPC IPv4 ranges, so it has no recorded
// fixture and is skipped during fixture playback, and it skips rather than fails when the account
// under test is not entitled. Record it with a suitably entitled token.
func TestLKECluster_Enterprise_AdditionalIPv4Ranges_smoke(t *testing.T) {
	if os.Getenv("LINODE_FIXTURE_MODE") == "play" {
		t.Skip("Skipping additional IPv4 ranges test: requires an entitled account, no fixture is recorded")
	}

	regionClient, regionTeardown := createTestClient(t, "fixtures/TestLKECluster_Enterprise_AdditionalIPv4Ranges_regions")

	regions := getRegionsWithCaps(t, regionClient, []linodego.RegionCapability{
		linodego.CapabilityLKE,
		linodego.CapabilityKubernetesEnterprise,
		linodego.CapabilityVPCCustomIPv4Ranges,
	})
	regionTeardown()

	if len(regions) == 0 {
		t.Skip("Skipping additional IPv4 ranges test: no region with custom VPC IPv4 range support")
	}

	requestedRanges := []linodego.LKEClusterAdditionalIPv4Range{
		{Range: "7.0.0.0/8"},
	}

	client, lkeCluster, teardown, err := setupLKECluster(t, []clusterModifier{func(createOpts *linodego.LKEClusterCreateOptions) {
		createOpts.Label = "go-lke-test-additional-ipv4-ranges"
		createOpts.Tier = "enterprise"
		createOpts.Region = regions[0]
		createOpts.K8sVersion = ""
		// LKE must own the workload VPC for this field to be accepted, so VpcID and SubnetID
		// are deliberately left unset.
		createOpts.AdditionalIPv4Ranges = requestedRanges
	}}, "fixtures/TestLKECluster_Enterprise_AdditionalIPv4Ranges_smoke")
	defer teardown()
	require.NoError(t, err)

	// The create response already reflects the request.
	require.Equal(t, requestedRanges, lkeCluster.AdditionalIPv4Ranges)

	cluster, err := client.GetLKECluster(context.Background(), lkeCluster.ID)
	require.NoErrorf(t, err, "Error getting lkeCluster: %v", err)
	require.Equal(t, lkeCluster.ID, cluster.ID)
	require.Equal(t, "enterprise", cluster.Tier)
	require.Equal(t, requestedRanges, cluster.AdditionalIPv4Ranges)

	// The same value is served by list.
	clusters, err := client.ListLKEClusters(context.Background(), nil)
	require.NoError(t, err)

	var listed *linodego.LKECluster

	for i := range clusters {
		if clusters[i].ID == lkeCluster.ID {
			listed = &clusters[i]
			break
		}
	}

	require.NotNil(t, listed, "created cluster was not returned by list")
	require.Equal(t, requestedRanges, listed.AdditionalIPv4Ranges)

	// The ranges are fixed once the cluster exists: an update that changes other fields must
	// leave them untouched.
	updated, err := client.UpdateLKECluster(context.Background(), lkeCluster.ID, linodego.LKEClusterUpdateOptions{
		Label: cluster.Label + "-updated",
	})
	require.NoError(t, err)
	require.Equal(t, requestedRanges, updated.AdditionalIPv4Ranges)
}

func TestLKECluster_Enterprise_BYOVPC_smoke(t *testing.T) {
	// bring your own vpc
	client, fixtureTeardown := createTestClient(t, "fixtures/TestLKECluster_Enterprise_VPC_smoke")

	regions := getRegionsWithCaps(t, client, []linodego.RegionCapability{
		linodego.CapabilityLKE,
		linodego.CapabilityDiskEncryption,
		linodego.CapabilityKubernetesEnterprise,
	})
	require.Greater(t, len(regions), 0, "Error getting regions with required capabilities")

	region := regions[0]
	vpc, _, vpcTeardown, err := createVPC(t, client, []vpcModifier{func(l *linodego.Client, options *linodego.VPCCreateOptions) {
		options.Region = region
		options.IPv6 = []linodego.VPCCreateOptionsIPv6{
			{
				Range: linodego.Pointer("/52"),
			},
		}
	}}...)
	require.NoErrorf(t, err, "Error creating VPC, got: %v", err)

	client, lkeCluster, teardown, err := setupLKECluster(t, []clusterModifier{func(createOpts *linodego.LKEClusterCreateOptions) {
		createOpts.Tier = "enterprise"
		createOpts.Region = region
		createOpts.K8sVersion = ""
		createOpts.VpcID = linodego.Pointer(vpc.ID)
		createOpts.StackType = linodego.Pointer(linodego.LKEClusterDualStack)
	}},
		"fixtures/TestLKECluster_Enterprise_BYOVPC_smoke")
	if err != nil {
		t.Errorf("Error creating lke, GOT ERROR %v", err)
	}

	defer func() {
		teardown()
		vpcTeardown()
		fixtureTeardown()
	}()

	cluster, err := client.GetLKECluster(context.Background(), lkeCluster.ID)
	if err != nil {
		t.Errorf("Error getting lkeCluster, expected struct, got %v and error %v", cluster, err)
	}
	if cluster.ID != lkeCluster.ID {
		t.Errorf("Expected a specific lkeCluster, but got a different one %v", cluster)
	}
	if cluster.VpcID != vpc.ID {
		t.Errorf("Expected an LKE cluster in VPC %v, but got in VPC %v.", vpc.ID, cluster.VpcID)
	}
	if cluster.StackType != linodego.LKEClusterDualStack {
		t.Errorf("Expected an LKE cluster stack_type is %v, but got %v.", linodego.LKEClusterDualStack, cluster.StackType)
	}
}

func TestLKECluster_Update(t *testing.T) {
	client, cluster, teardown, err := setupLKECluster(t, []clusterModifier{func(createOpts *linodego.LKEClusterCreateOptions) {
		createOpts.Label = "go-lke-test-update"
	}}, "fixtures/TestLKECluster_Update")
	defer teardown()
	if err != nil {
		t.Fatal(err)
	}

	updatedTags := []string{"test=true"}
	updatedLabel := cluster.Label + "-updated"

	updatedCluster, err := client.UpdateLKECluster(context.Background(), cluster.ID, linodego.LKEClusterUpdateOptions{
		Tags:  updatedTags,
		Label: updatedLabel,
	})
	if err != nil {
		t.Fatalf("failed to update LKE Cluster (%d): %s", cluster.ID, err)
	}

	if updatedCluster.Label != updatedLabel {
		t.Errorf("expected label to be updated to %q; got %q", updatedLabel, updatedCluster.Label)
	}

	if !reflect.DeepEqual(updatedTags, updatedCluster.Tags) {
		t.Errorf("expected tags to be updated to %#v; got %#v", updatedTags, updatedCluster.Tags)
	}

	// Update the LKE cluster to HA
	// This needs to be done in a separate API request from the K8s version upgrade
	isHA := true
	updatedControlPlane := &linodego.LKEClusterControlPlaneOptions{HighAvailability: &isHA}

	updatedCluster, err = client.UpdateLKECluster(context.Background(), cluster.ID, linodego.LKEClusterUpdateOptions{
		ControlPlane: updatedControlPlane,
	})
	if err != nil {
		t.Fatalf("failed to update LKE Cluster (%d): %s", cluster.ID, err)
	}

	if !reflect.DeepEqual(*updatedControlPlane.HighAvailability, updatedCluster.ControlPlane.HighAvailability) {
		t.Errorf("expected control plane to be updated to %#v; got %#v", updatedControlPlane, updatedCluster.ControlPlane)
	}
}

func TestLKECluster_Nodes_Recycle(t *testing.T) {
	client, cluster, teardown, err := setupLKECluster(t, []clusterModifier{func(createOpts *linodego.LKEClusterCreateOptions) {
		createOpts.Label = "go-lke-test-recycle"
	}}, "fixtures/TestLKECluster_Nodes_Recycle")
	defer teardown()
	if err != nil {
		t.Fatal(err)
	}

	err = client.RecycleLKEClusterNodes(context.TODO(), cluster.ID)
	if err != nil {
		t.Errorf("failed to recycle LKE cluster: %s", err)
	}
}

func TestLKECluster_APIEndpoints_List(t *testing.T) {
	client, lkeCluster, teardown, err := setupLKECluster(t, []clusterModifier{func(createOpts *linodego.LKEClusterCreateOptions) {
		createOpts.Label = "go-lke-test-apiend"
	}}, "fixtures/TestLKECluster_APIEndpoints_List")
	defer teardown()

	if err != nil {
		t.Error(err)
	}

	i, err := client.ListLKEClusterAPIEndpoints(context.Background(), lkeCluster.ID, nil)
	if err != nil {
		t.Errorf("Error listing lkeClusterAPIEndpoints, expected struct, got error %v", err)
	}
	if len(i) <= 0 {
		t.Errorf("Expected some lkeClusterAPIEndpoints, but got none %v", i)
	}
}

func TestLKECluster_Kubeconfig_Get(t *testing.T) {
	ctx := waitContext(t, 180*time.Second)

	client, lkeCluster, teardown, err := setupLKECluster(t, []clusterModifier{func(createOpts *linodego.LKEClusterCreateOptions) {
		createOpts.Label = "go-lke-test-kube-get"
	}}, "fixtures/TestLKECluster_Kubeconfig_Get")
	defer teardown()

	_, err = client.WaitForLKEClusterStatus(ctx, lkeCluster.ID, linodego.LKEClusterReady)
	if err != nil {
		t.Errorf("Error waiting for LKECluster readiness: %s", err)
	}
	i, err := client.GetLKEClusterKubeconfig(context.Background(), lkeCluster.ID)
	if err != nil {
		t.Errorf("Error getting lkeCluster Kubeconfig, expected struct, got %v and error %v", i, err)
	}
	if len(i.KubeConfig) == 0 {
		t.Errorf("Expected an lkeCluster Kubeconfig, but got empty string %v", i)
	}
}

func TestLKECluster_Kubeconfig_Delete(t *testing.T) {
	ctx := waitContext(t, 180*time.Second)

	client, lkeCluster, teardown, err := setupLKECluster(t, []clusterModifier{func(createOpts *linodego.LKEClusterCreateOptions) {
		createOpts.Label = "go-lke-test-kube-delete"
	}}, "fixtures/TestLKECluster_Kubeconfig_Delete")
	defer teardown()

	_, err = client.WaitForLKEClusterStatus(ctx, lkeCluster.ID, linodego.LKEClusterReady)
	if err != nil {
		t.Errorf("Error waiting for LKECluster readiness: %s", err)
	}
	i, err := client.GetLKEClusterKubeconfig(context.Background(), lkeCluster.ID)
	if err != nil {
		t.Errorf("Error getting lkeCluster Kubeconfig, expected struct, got %v and error %v", i, err)
	}
	if len(i.KubeConfig) == 0 {
		t.Errorf("Expected an lkeCluster Kubeconfig, but got empty string %v", i)
	}

	delete_err := client.DeleteLKEClusterKubeconfig(context.Background(), lkeCluster.ID)
	if err != nil {
		t.Errorf("Error deleting lkeCluster Kubeconfig, got error %v", delete_err)
	}
}

func TestLKEClusters_List(t *testing.T) {
	client, _, teardown, err := setupLKECluster(t, []clusterModifier{func(createOpts *linodego.LKEClusterCreateOptions) {
		createOpts.Label = "go-lke-test-list"
	}}, "fixtures/TestLKEClusters_List")
	if err != nil {
		t.Error(err)
	}
	defer teardown()

	// @TODO filter on the known label, API docs say this is supported, but it
	// errors
	i, err := client.ListLKEClusters(context.Background(), nil)
	if err != nil {
		t.Errorf("Error listing lkeClusters, expected struct, got error %v", err)
	}
	if len(i) == 0 {
		t.Errorf("Expected a list of lkeClusters, but got none %v", i)
	}
}

func TestLKEVersion_GetMissing(t *testing.T) {
	client, teardown := createTestClient(t, "fixtures/TestLKEVersion_GetMissing")
	defer teardown()

	i, err := client.GetLKEVersion(context.Background(), "does-not-exist")
	if err == nil {
		t.Errorf("should have received an error requesting a missing version, got %v", i)
	}
	e, ok := err.(*linodego.Error)
	if !ok {
		t.Errorf("should have received an Error requesting a missing version, got %v", e)
	}

	if e.Code != 404 {
		t.Errorf("should have received a 404 Code requesting a missing version, got %v", e.Code)
	}
}

func TestLKEVersion_GetFound(t *testing.T) {
	client, teardown := createTestClient(t, "fixtures/TestLKEVersion_GetFound")
	defer teardown()

	i, err := client.GetLKEVersion(context.Background(), "1.29")
	if err != nil {
		t.Errorf("Error getting version, expected struct, got %v and error %v", i, err)
	}

	if i.ID != "1.29" {
		t.Errorf("Expected a specific version, but got a different one %v", i)
	}
}

func TestLKEVersions_List(t *testing.T) {
	client, teardown := createTestClient(t, "fixtures/TestLKEVersions_List")
	defer teardown()

	i, err := client.ListLKEVersions(context.Background(), nil)
	if err != nil {
		t.Errorf("Error listing versions, expected struct, got error %v", err)
	}
	if len(i) == 0 {
		t.Errorf("Expected a list of versions, but got none %v", i)
	}
}

func TestLKECluster_APLEnabled_smoke(t *testing.T) {
	client, lkeCluster, teardown, err := setupLKECluster(t, []clusterModifier{
		func(createOpts *linodego.LKEClusterCreateOptions) {
			createOpts.Label = "go-lke-test-apl-enabled"
		},
		func(createOpts *linodego.LKEClusterCreateOptions) {
			createOpts.APLEnabled = true
		},
		func(createOpts *linodego.LKEClusterCreateOptions) {
			// NOTE: g6-dedicated-4 is the minimum APL-compatible Linode type
			createOpts.NodePools = []linodego.LKENodePoolCreateOptions{{Count: 3, Type: "g6-dedicated-4", Tags: []string{"test"}}}
		},
	},
		"fixtures/TestLKECluster_APLEnabled")
	defer teardown()

	expectedConsoleURL := fmt.Sprintf("https://console.lke%d.akamai-apl.net", lkeCluster.ID)
	consoleURL, err := client.GetLKEClusterAPLConsoleURL(context.Background(), lkeCluster.ID)
	if err != nil {
		t.Errorf("Error getting LKE APL console URL, expected string, got %v and error %v", consoleURL, err)
	}
	if consoleURL != expectedConsoleURL {
		t.Errorf("Expected an APL console URL %v, but got a different one %v", expectedConsoleURL, consoleURL)
	}

	expectedHealthCheckURL := fmt.Sprintf("https://auth.lke%d.akamai-apl.net/ready", lkeCluster.ID)
	healthCheckURL, err := client.GetLKEClusterAPLHealthCheckURL(context.Background(), lkeCluster.ID)
	if err != nil {
		t.Errorf("Error getting LKE APL health check URL, expected string, got %v and error %v", healthCheckURL, err)
	}
	if healthCheckURL != expectedHealthCheckURL {
		t.Errorf("Expected an APL health check URL %v, but got a different one %v", expectedHealthCheckURL, healthCheckURL)
	}
}

func TestLKETierVersion_ListAndGet(t *testing.T) {
	client, teardown := createTestClient(t, "fixtures/TestLKETierVersion_ListAndGet")
	defer teardown()

	testCases := []string{"standard", "enterprise"}

	for _, tier := range testCases {
		t.Run(fmt.Sprintf("Tier=%s", tier), func(t *testing.T) {
			versions, err := client.ListLKETierVersions(context.Background(), tier, nil)
			if err != nil {
				t.Fatalf("Error listing versions: %v", err)
			}

			if len(versions) == 0 {
				t.Fatalf("Expected a list of versions for tier %s, but got none", tier)
			}

			for _, version := range versions {
				if string(version.Tier) != tier {
					t.Errorf("Expected version tier %q, but got %q", tier, version.Tier)
				}
			}

			v, err := client.GetLKETierVersion(context.Background(), tier, versions[0].ID)
			if err != nil {
				t.Fatalf("Error getting version %s for tier %s: %v", versions[0].ID, tier, err)
			}

			if v.ID != versions[0].ID {
				t.Errorf("Expected version ID %q, but got %q", versions[0].ID, v.ID)
			}
		})
	}
}

type clusterModifier func(*linodego.LKEClusterCreateOptions)

func setupLKECluster(t *testing.T, clusterModifiers []clusterModifier, fixturesYaml string) (*linodego.Client, *linodego.LKECluster, func(), error) {
	t.Helper()
	var fixtureTeardown func()
	client, fixtureTeardown := createTestClient(t, fixturesYaml)

	createOpts := linodego.LKEClusterCreateOptions{
		Label:      label,
		Tier:       "standard", // default, can be overridden
		Tags:       []string{"testing"},
		Region:     "", // region will be resolved below
		K8sVersion: "", // will be resolved if empty
		NodePools: []linodego.LKENodePoolCreateOptions{{
			Count: 1,
			Type:  "g6-standard-2",
			Tags:  []string{"test"},
		}},
		VpcID:     nil, // default, overridden if needed
		SubnetID:  nil, // default, overridden if needed
		StackType: nil, // default, overridden if needed
	}

	for _, modifier := range clusterModifiers {
		modifier(&createOpts)
	}

	if createOpts.Region == "" {
		createOpts.Region = getRegionsWithCaps(t, client, []linodego.RegionCapability{
			linodego.CapabilityLKE,
			linodego.CapabilityLADiskEncryption,
		})[0]
	}

	if createOpts.K8sVersion == "" {
		createOpts.K8sVersion = getK8sVersion(t, client, createOpts.Tier)
	}

	lkeCluster, err := client.CreateLKECluster(context.Background(), createOpts)
	if err != nil {
		t.Errorf("failed to create LKE cluster: %s", err)
	}

	teardown := func() {
		if err := client.DeleteLKECluster(context.Background(), lkeCluster.ID); err != nil {
			t.Errorf("failed to delete LKE cluster: %s", err)
		}
		fixtureTeardown()
	}
	return client, lkeCluster, teardown, err
}

func getK8sVersion(t *testing.T, client *linodego.Client, tier string) string {
	t.Helper()

	versions, err := client.ListLKETierVersions(context.Background(), tier, nil)
	if err != nil {
		t.Fatalf("Error listing versions for tier %q: %v", tier, err)
	}

	if len(versions) == 0 {
		t.Fatalf("Expected a list of versions for tier %q, but got none", tier)
	}

	return versions[0].ID
}
