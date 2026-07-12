package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &DashboardRoleDataSource{}

func newDashboardRoleDataSource() datasource.DataSource {
	return &DashboardRoleDataSource{}
}

type DashboardRoleDataSource struct {
	client *restapi.Client
}

type DashboardRoleDataSourceModel struct {
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	ScopesPermissions types.Map    `tfsdk:"scopes_permissions"`
	System            types.Bool   `tfsdk:"system"`
}

func (d *DashboardRoleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard_role"
}

func (d *DashboardRoleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a Ceph dashboard role, including built-in system roles.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the role",
				Required:            true,
			},
			"description": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The description of the role",
				Computed:            true,
			},
			"scopes_permissions": dataSourceSchema.MapAttribute{
				MarkdownDescription: "The permissions granted per security scope",
				Computed:            true,
				ElementType:         types.SetType{ElemType: types.StringType},
			},
			"system": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether this is a built-in system role",
				Computed:            true,
			},
		},
	}
}

func (d *DashboardRoleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*restapi.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *restapi.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *DashboardRoleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DashboardRoleDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	role, err := d.client.GetDashboardRole(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get dashboard role '%s' from Ceph API: %s", data.Name.ValueString(), err),
		)
		return
	}

	data.Description = types.StringPointerValue(role.Description)
	data.System = types.BoolValue(role.System)

	scopesPermissions, diags := types.MapValueFrom(ctx, types.SetType{ElemType: types.StringType}, role.ScopesPermissions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ScopesPermissions = scopesPermissions

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
