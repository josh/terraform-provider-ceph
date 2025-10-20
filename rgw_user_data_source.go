package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RGWUserDataSource{}

func newRGWUserDataSource() datasource.DataSource {
	return &RGWUserDataSource{}
}

type RGWUserDataSource struct {
	client *CephAPIClient
}

type RGWUserDataSourceModel struct {
	UID         types.String `tfsdk:"uid"`
	UserID      types.String `tfsdk:"user_id"`
	DisplayName types.String `tfsdk:"display_name"`
	Email       types.String `tfsdk:"email"`
	MaxBuckets  types.Int64  `tfsdk:"max_buckets"`
	System      types.Bool   `tfsdk:"system"`
	Suspended   types.Bool   `tfsdk:"suspended"`
	Tenant      types.String `tfsdk:"tenant"`
	Admin       types.Bool   `tfsdk:"admin"`
}

func (d *RGWUserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_user"
}

func (d *RGWUserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a Ceph RGW user.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"uid": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The user ID",
				Required:            true,
			},
			"user_id": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The user ID returned by the API",
				Computed:            true,
			},
			"display_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The display name of the user",
				Computed:            true,
			},
			"email": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The email address of the user",
				Computed:            true,
			},
			"max_buckets": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "Maximum number of buckets the user can own",
				Computed:            true,
			},
			"system": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether this is a system user",
				Computed:            true,
			},
			"suspended": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether this user is suspended",
				Computed:            true,
			},
			"tenant": dataSourceSchema.StringAttribute{
				MarkdownDescription: "Tenant for multi-tenancy support",
				Computed:            true,
			},
			"admin": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether this user has admin privileges (read-only, can only be set via radosgw-admin CLI)",
				Computed:            true,
			},
		},
	}
}

func (d *RGWUserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *CephAPIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *RGWUserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RGWUserDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	uid := data.UID.ValueString()
	user, err := d.client.RGWGetUser(ctx, uid)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get RGW user from Ceph API: %s", err),
		)
		return
	}

	data.UserID = types.StringValue(user.UserID)
	data.DisplayName = types.StringValue(user.DisplayName)
	data.Email = types.StringValue(user.Email)
	data.MaxBuckets = types.Int64Value(int64(user.MaxBuckets))
	data.System = types.BoolValue(user.System)
	data.Suspended = types.BoolValue(user.Suspended == 1)
	data.Tenant = types.StringValue(user.Tenant)
	data.Admin = types.BoolValue(user.Admin)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
