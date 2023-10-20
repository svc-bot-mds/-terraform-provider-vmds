package mds

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/svc-bot-mds/terraform-provider-vmds/client/constants/service_type"
	"github.com/svc-bot-mds/terraform-provider-vmds/client/mds"
	"github.com/svc-bot-mds/terraform-provider-vmds/client/mds/controller"
	"github.com/svc-bot-mds/terraform-provider-vmds/client/mds/core"
	"github.com/svc-bot-mds/terraform-provider-vmds/client/model"
	"net/http"
	"time"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &clusterResource{}
	_ resource.ResourceWithConfigure   = &clusterResource{}
	_ resource.ResourceWithImportState = &clusterResource{}
)

func NewClusterResource() resource.Resource {
	return &clusterResource{}
}

type clusterResource struct {
	client *mds.Client
}

// clusterResourceModel maps the resource schema data.
type clusterResourceModel struct {
	ID               types.String `tfsdk:"id"`
	OrgId            types.String `tfsdk:"org_id"`
	Name             types.String `tfsdk:"name"`
	ServiceType      types.String `tfsdk:"service_type"`
	Provider         types.String `tfsdk:"cloud_provider"`
	InstanceSize     types.String `tfsdk:"instance_size"`
	Region           types.String `tfsdk:"region"`
	Tags             types.Set    `tfsdk:"tags"`
	NetworkPolicyIds types.Set    `tfsdk:"network_policy_ids"`
	Dedicated        types.Bool   `tfsdk:"dedicated"`
	Shared           types.Bool   `tfsdk:"shared"`
	Status           types.String `tfsdk:"status"`
	DataPlaneId      types.String `tfsdk:"data_plane_id"`
	LastUpdated      types.String `tfsdk:"last_updated"`
	Created          types.String `tfsdk:"created"`
	Metadata         types.Object `tfsdk:"metadata"`
	// TODO add upgrade related fields
}

// clusterMetadataModel maps order item data.
type clusterMetadataModel struct {
	ManagerUri       types.String `tfsdk:"manager_uri"`
	ConnectionUri    types.String `tfsdk:"connection_uri"`
	MetricsEndpoints types.List   `tfsdk:"metrics_endpoints"`
}

func (r *clusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *clusterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*mds.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *mds.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

// Schema defines the schema for the resource.
func (r *clusterResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	tflog.Info(ctx, "INIT__Schema")

	resp.Schema = schema.Schema{
		MarkdownDescription: "Represents a service instance or cluster. Some attributes are used only once for creation, they are: `dedicated`, `network_policy_ids`." +
			"\nChanging only `tags` is supported at the moment. If you wish to update network policies associated with it, please refer resource: " +
			"`vmds_cluster_network_policies_association`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "ID of the cluster.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_id": schema.StringAttribute{
				Description: "ID of the Org which owns the cluster.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the cluster.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_type": schema.StringAttribute{
				MarkdownDescription: fmt.Sprintf("Type of MDS Cluster to be created. Supported values: %s .\n Default is `RABBITMQ`.", supportedServiceTypesMarkdown()),
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(service_type.RABBITMQ),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cloud_provider": schema.StringAttribute{
				MarkdownDescription: "Short-code of provider to use for data-plane. Ex: `aws`, `gcp` .",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"instance_size": schema.StringAttribute{
				MarkdownDescription: "Size of instance. Supported values are: `XX-SMALL`, `X-SMALL`, `SMALL`, `LARGE`, `XX-LARGE`." +
					"\nPlease make use of datasource `vmds_network_ports` to decide on a size based on resources it requires.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Region of data plane. Ex: `eu-west-2`, `us-east-2` etc.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"dedicated": schema.BoolAttribute{
				Description: "If present and set to `true`, the cluster will get deployed on a dedicated data-plane in current Org.",
				Optional:    true,
				Computed:    false,
			},
			"shared": schema.BoolAttribute{
				Description: "If present and set to `true`, the cluster will get deployed on a shared data-plane in current Org.",
				Optional:    true,
				Computed:    false,
			},
			"tags": schema.SetAttribute{
				Description: "Set of tags or labels to categorise the cluster.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"network_policy_ids": schema.SetAttribute{
				Description: "IDs of network policies to attach to the cluster.",
				Required:    true,
				Computed:    false,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Description: "Status of the cluster.",
				Computed:    true,
			},
			"data_plane_id": schema.StringAttribute{
				Description: "ID of the data-plane where the cluster is running. It's a required field when we create a cluster which is self-hosted via BYO Cloud",
				Computed:    true,
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"last_updated": schema.StringAttribute{
				Description: "Time when the cluster was last modified.",
				Computed:    true,
			},
			"created": schema.StringAttribute{
				Description: "Creation time of the cluster.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"metadata": schema.SingleNestedAttribute{
				Description: "Additional info of the cluster.",
				CustomType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"manager_uri":    types.StringType,
						"connection_uri": types.StringType,
						"metrics_endpoints": types.ListType{
							ElemType: types.StringType,
						},
					},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"manager_uri": schema.StringAttribute{
						MarkdownDescription: "URI of the manager. Specific to `RABBITMQ` service.",
						Computed:            true,
					},
					"connection_uri": schema.StringAttribute{
						MarkdownDescription: "Connection URI to the instance. Specific to `RABBITMQ` service.",
						Computed:            true,
					},
					"metrics_endpoints": schema.ListAttribute{
						MarkdownDescription: "List of metrics endpoints exposed on the instance. Specific to `RABBITMQ` service.",
						Computed:            true,
						ElementType:         types.StringType,
					},
				},
			},
		},
	}

	tflog.Info(ctx, "END__Schema")
}

// Create a new resource
func (r *clusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Info(ctx, "INIT__Create")
	// Retrieve values from plan
	var plan clusterResourceModel
	diags := req.Plan.Get(ctx, &plan)
	if resp.Diagnostics.Append(diags...); resp.Diagnostics.HasError() {
		return
	}

	// Generate API request body from plan
	clusterRequest := controller.MdsClusterCreateRequest{
		Name:         plan.Name.ValueString(),
		ServiceType:  plan.ServiceType.ValueString(),
		InstanceSize: plan.InstanceSize.ValueString(),
		Provider:     plan.Provider.ValueString(),
		Region:       plan.Region.ValueString(),
		Dedicated:    plan.Dedicated.ValueBool(),
		Shared:       plan.Shared.ValueBool(),
		DataPlaneId:  plan.DataPlaneId.ValueString(),
	}
	plan.Tags.ElementsAs(ctx, &clusterRequest.Tags, true)
	plan.NetworkPolicyIds.ElementsAs(ctx, &clusterRequest.NetworkPolicyIds, true)

	if _, err := r.client.Controller.CreateMdsCluster(&clusterRequest); err != nil {
		resp.Diagnostics.AddError(
			"Submitting request to create cluster",
			"Could not create cluster, unexpected error: "+err.Error(),
		)
		return
	}

	clusters, err := r.client.Controller.GetMdsClusters(&controller.MdsClustersQuery{
		ServiceType:   clusterRequest.ServiceType,
		Name:          clusterRequest.Name,
		FullNameMatch: true,
	})
	if err != nil {
		resp.Diagnostics.AddError("Fetching clusters",
			"Could not fetch clusters by name, unexpected error: "+err.Error(),
		)
		return
	}

	if len(*clusters.Get()) <= 0 {
		resp.Diagnostics.AddError("Fetching Clusters",
			"Unable to fetch the created cluster",
		)
		return
	}

	// Map response body to schema and populate Computed attribute values
	createdCluster := &(*clusters.Get())[0]
	if createdCluster.Status == "FAILED" {
		resp.Diagnostics.AddError("Error creating cluster",
			"Cluster creation failed with the status 'FAILED'")
		return
	} else {
		for createdCluster.Status != "READY" && createdCluster.Status != "FAILED" {
			time.Sleep(10 * time.Second)
			createdCluster, err = r.client.Controller.GetMdsCluster(createdCluster.ID)
			if err != nil {
				resp.Diagnostics.AddError("Fetching cluster",
					"Could not fetch cluster by ID, unexpected error: "+err.Error(),
				)
				return
			}
		}
	}
	if saveFromResponse(&ctx, &resp.Diagnostics, &plan, createdCluster) != 0 {
		return
	}

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "END__Create")
}

// Read resource information
func (r *clusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	tflog.Info(ctx, "INIT__Read")
	// Get current state
	var state clusterResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get refreshed cluster value from MDS
	cluster, err := r.client.Controller.GetMdsCluster(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Reading MDS Cluster",
			"Could not read MDS cluster ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Overwrite items with refreshed state
	if saveFromResponse(&ctx, &resp.Diagnostics, &state, cluster) != 0 {
		return
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "END__Read")
}

func (r *clusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	tflog.Info(ctx, "INIT__Update")

	// Retrieve values from plan
	var plan clusterResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate API request body from plan
	var updateRequest controller.MdsClusterUpdateRequest
	plan.Tags.ElementsAs(ctx, &updateRequest.Tags, true)

	// Update existing cluster
	cluster, err := r.client.Controller.UpdateMdsCluster(plan.ID.ValueString(), &updateRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Updating MDS Cluster",
			"Could not update cluster, unexpected error: "+err.Error(),
		)
		return
	}

	// Update resource state with updated items and timestamp
	if saveFromResponse(&ctx, &resp.Diagnostics, &plan, cluster) != 0 {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "END__Update")
}

func (r *clusterResource) Delete(ctx context.Context, request resource.DeleteRequest, resp *resource.DeleteResponse) {
	tflog.Info(ctx, "INIT__Delete")
	// Get current state
	var state clusterResourceModel
	diags := request.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Submit request to delete MDS Cluster
	_, err := r.client.Controller.DeleteMdsCluster(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Deleting MDS Cluster",
			"Could not delete MDS cluster by ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	for {
		time.Sleep(10 * time.Second)
		if _, err := r.client.Controller.GetMdsCluster(state.ID.ValueString()); err != nil {
			if err != nil {
				var apiError core.ApiError
				if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
					break
				}
				resp.Diagnostics.AddError("Fetching cluster",
					fmt.Sprintf("Could not fetch cluster by id [%v], unexpected error: %s", state.ID, err.Error()),
				)
				return
			}
		}
	}

	tflog.Info(ctx, "END__Delete")
}

func (r *clusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Retrieve import ID and save to id attribute
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func saveFromResponse(ctx *context.Context, diagnostics *diag.Diagnostics, state *clusterResourceModel, cluster *model.MdsCluster) int8 {
	tflog.Info(*ctx, "Saving response to resourceModel state/plan")
	state.ID = types.StringValue(cluster.ID)
	state.Name = types.StringValue(cluster.Name)
	state.ServiceType = types.StringValue(cluster.ServiceType)
	state.Provider = types.StringValue(cluster.Provider)
	state.InstanceSize = types.StringValue(cluster.InstanceSize)
	state.Region = types.StringValue(cluster.Region)
	state.Status = types.StringValue(cluster.Status)
	state.OrgId = types.StringValue(cluster.OrgId)
	state.DataPlaneId = types.StringValue(cluster.DataPlaneId)
	state.LastUpdated = types.StringValue(cluster.LastUpdated)
	state.Created = types.StringValue(cluster.Created)
	tflog.Info(*ctx, "trying to save mdsMetadata", map[string]interface{}{
		"obj": cluster.Metadata,
	})

	metadataModel := clusterMetadataModel{
		ManagerUri:       types.StringValue(""),
		ConnectionUri:    types.StringValue(""),
		MetricsEndpoints: types.ListNull(types.StringType),
	}
	if cluster.Metadata != nil {
		list, diags := types.ListValueFrom(*ctx, types.StringType, cluster.Metadata.MetricsEndpoints)
		if diagnostics.Append(diags...); diagnostics.HasError() {
			return 1
		}
		metadataModel.ManagerUri = types.StringValue(cluster.Metadata.ManagerUri)
		metadataModel.ConnectionUri = types.StringValue(cluster.Metadata.ConnectionUri)
		metadataModel.MetricsEndpoints = list
	}
	metadataObject, diags := types.ObjectValueFrom(*ctx, state.Metadata.AttributeTypes(*ctx), metadataModel)
	if diagnostics.Append(diags...); diagnostics.HasError() {
		return 1
	}
	state.Metadata = metadataObject

	list, diags := types.SetValueFrom(*ctx, types.StringType, cluster.Tags)
	if diagnostics.Append(diags...); diagnostics.HasError() {
		return 1
	}
	state.Tags = list
	return 0
}
