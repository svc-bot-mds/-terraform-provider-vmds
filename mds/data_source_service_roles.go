package mds

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/svc-bot-mds/terraform-provider-vmds/client/constants/role_type"
	"github.com/svc-bot-mds/terraform-provider-vmds/client/mds"
	service_metadata "github.com/svc-bot-mds/terraform-provider-vmds/client/mds/service-metadata"
)

var (
	_ datasource.DataSource              = &serviceRolesDatasource{}
	_ datasource.DataSourceWithConfigure = &serviceRolesDatasource{}
)

// ServiceRolesDataSourceModel maps the data source schema data.
type ServiceRolesDataSourceModel struct {
	Roles []ServiceRolesModel `tfsdk:"roles"`
	Type  types.String        `tfsdk:"type"`
}

// ServiceRolesModel maps role schema data.
type ServiceRolesModel struct {
	RoleId      types.String                     `tfsdk:"role_id"`
	Name        types.String                     `tfsdk:"name"`
	Description types.String                     `tfsdk:"description"`
	Type        types.String                     `tfsdk:"type"`
	Permissions []MdsServiceRolePermissionsModel `tfsdk:"permissions"`
}

type MdsServiceRolePermissionsModel struct {
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	PermissionId types.String `tfsdk:"permission_id"`
}

// NewServiceRolesDatasource is a helper function to simplify the provider implementation.
func NewServiceRolesDatasource() datasource.DataSource {
	return &serviceRolesDatasource{}
}

// rolesDatasource is the data source implementation.
type serviceRolesDatasource struct {
	client *mds.Client
}

// Metadata returns the data source type name.
func (d *serviceRolesDatasource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_roles"

}

// Schema defines the schema for the data source.
func (d *serviceRolesDatasource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Required: true,
			},
			"roles": schema.ListNestedAttribute{
				Computed: true,
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"role_id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"description": schema.StringAttribute{
							Computed: true,
						},
						"type": schema.StringAttribute{
							Computed: true,
						},
						"permissions": schema.ListNestedAttribute{
							Computed: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"description": schema.StringAttribute{
										Computed: true,
									},
									"name": schema.StringAttribute{
										Computed: true,
									},
									"permission_id": schema.StringAttribute{
										Computed: true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (d *serviceRolesDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state ServiceRolesDataSourceModel
	tflog.Info(ctx, "INIT -- READ service roles")
	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)

	query := &service_metadata.MDSRolesQuery{
		Type: role_type.RABBITMQ,
	}
	rolesResponse, err := d.client.ServiceMetadata.GetMdsServiceRoles(query)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read MDS Service roles",
			err.Error(),
		)
		return
	}
	var permissionsVal []MdsServiceRolePermissionsModel
	for _, role := range rolesResponse.Embedded.ServiceRoleDTO[0].Roles {
		for _, permissionList := range role.Permissions {
			permission := MdsServiceRolePermissionsModel{
				Name:         types.StringValue(permissionList.Name),
				Description:  types.StringValue(permissionList.Description),
				PermissionId: types.StringValue(permissionList.PermissionId),
			}
			permissionsVal = append(permissionsVal, permission)
		}
		// Extract the roles from the unmarshalled struct
		for _, role := range rolesResponse.Embedded.ServiceRoleDTO[0].Roles {
			roleList := ServiceRolesModel{
				RoleId:      types.StringValue(role.RoleID),
				Name:        types.StringValue(role.Name),
				Description: types.StringValue(role.Description),
				Type:        types.StringValue(role.Type),
				Permissions: permissionsVal,
			}
			state.Roles = append(state.Roles, roleList)
		}
	}

	// Set state
	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Configure adds the provider configured client to the data source.
func (d *serviceRolesDatasource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.client = req.ProviderData.(*mds.Client)
}
