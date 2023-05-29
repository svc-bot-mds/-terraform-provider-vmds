package mds

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/svc-bot-mds/terraform-provider-vmds/client/mds"
	infra_connector "github.com/svc-bot-mds/terraform-provider-vmds/client/mds/infra-connector"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &regionsDataSource{}
	_ datasource.DataSourceWithConfigure = &regionsDataSource{}
)

type regionsDataSourceModel struct {
	Cpu                types.String   `tfsdk:"cpu"`
	Provider           types.String   `tfsdk:"cloud_provider"`
	Memory             types.String   `tfsdk:"memory"`
	Storage            types.String   `tfsdk:"storage"`
	NodeCount          types.String   `tfsdk:"node_count"`
	Regions            []RegionsModel `tfsdk:"regions"`
	DedicatedDataPlane types.Bool     `tfsdk:"dedicated_data_plane"`
	Id                 types.String   `tfsdk:"id"`
}

type RegionsModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	DataPlaneIds types.List   `tfsdk:"data_plane_ids"`
}

func NewRegionsDataSource() datasource.DataSource {
	return &regionsDataSource{}
}

type regionsDataSource struct {
	client *mds.Client
}

// Metadata returns the data source type name.
func (d *regionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_regions"
}

func (d *regionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"cpu": schema.StringAttribute{
				Required: true,
			},
			"cloud_provider": schema.StringAttribute{
				Required: true,
			},
			"memory": schema.StringAttribute{
				Required: true,
			},
			"storage": schema.StringAttribute{
				Required: true,
			},
			"node_count": schema.StringAttribute{
				Required: true,
			},
			"dedicated_data_plane": schema.BoolAttribute{
				Optional: true,
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The testing framework requires an id attribute to be present in every data source and resource",
			},
			"regions": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"data_plane_ids": schema.ListAttribute{
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *regionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.client = req.ProviderData.(*mds.Client)
}

// Read refreshes the Terraform state with the latest data.
func (d *regionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state regionsDataSourceModel

	//Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)

	regionQuery := &infra_connector.DataPlaneRegionsQuery{
		CPU:       state.Cpu.ValueString(),
		NodeCount: state.NodeCount.ValueString(),
		Memory:    state.Memory.ValueString(),
		Storage:   state.Storage.ValueString(),
		Provider:  state.Provider.ValueString(),
	}
	if state.DedicatedDataPlane.ValueBool() {
		regionQuery.OrgId = d.client.Root.OrgId
	}
	regions, err := d.client.InfraConnector.GetRegionsWithDataPlanes(regionQuery)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read MDS Regions:",
			err.Error(),
		)
		return
	}
	if saveRegionsToState(&ctx, &resp.Diagnostics, &state, regions) != 0 {
		return
	}
	state.Id = types.StringValue("placeholder")
	// Set state
	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

}

func saveRegionsToState(ctx *context.Context, diagnostics *diag.Diagnostics, state *regionsDataSourceModel, regions map[string][]string) int8 {
	for regionName, dataPlaneIds := range regions {
		instanceTypesState := RegionsModel{
			ID:   types.StringValue(regionName),
			Name: types.StringValue(regionName),
		}
		list, diags := types.ListValueFrom(*ctx, types.StringType, dataPlaneIds)
		if diagnostics.Append(diags...); diagnostics.HasError() {
			return 1
		}
		instanceTypesState.DataPlaneIds = list
		state.Regions = append(state.Regions, instanceTypesState)
	}
	return 0
}
