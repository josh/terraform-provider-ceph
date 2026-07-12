package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &DashboardUserDataSource{}

func newDashboardUserDataSource() datasource.DataSource {
	return &DashboardUserDataSource{}
}

type DashboardUserDataSource struct {
	client *restapi.Client
}

type DashboardUserDataSourceModel struct {
	Username          types.String `tfsdk:"username"`
	Name              types.String `tfsdk:"name"`
	Email             types.String `tfsdk:"email"`
	Roles             types.Set    `tfsdk:"roles"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	PwdExpirationDate types.Int64  `tfsdk:"pwd_expiration_date"`
	PwdUpdateRequired types.Bool   `tfsdk:"pwd_update_required"`
	LastUpdate        types.Int64  `tfsdk:"last_update"`
}

func (d *DashboardUserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard_user"
}

func (d *DashboardUserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a Ceph dashboard user.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"username": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The username of the dashboard user",
				Required:            true,
			},
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The full name of the user",
				Computed:            true,
			},
			"email": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The email address of the user",
				Computed:            true,
			},
			"roles": dataSourceSchema.SetAttribute{
				MarkdownDescription: "The roles assigned to the user",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"enabled": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the user account is enabled",
				Computed:            true,
			},
			"pwd_expiration_date": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The password expiration date as epoch seconds",
				Computed:            true,
			},
			"pwd_update_required": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the user must change their password",
				Computed:            true,
			},
			"last_update": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The last modification time of the user as epoch seconds",
				Computed:            true,
			},
		},
	}
}

func (d *DashboardUserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DashboardUserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DashboardUserDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	user, err := d.client.GetDashboardUser(ctx, data.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get dashboard user '%s' from Ceph API: %s", data.Username.ValueString(), err),
		)
		return
	}

	data.Name = types.StringPointerValue(user.Name)
	data.Email = types.StringPointerValue(user.Email)
	data.Enabled = types.BoolValue(user.Enabled)
	data.PwdExpirationDate = types.Int64PointerValue(user.PwdExpirationDate)
	data.PwdUpdateRequired = types.BoolValue(user.PwdUpdateRequired)
	data.LastUpdate = types.Int64Value(user.LastUpdate)

	roles, diags := types.SetValueFrom(ctx, types.StringType, user.Roles)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Roles = roles

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
