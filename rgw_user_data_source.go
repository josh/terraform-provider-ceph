package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &RGWUserDataSource{}

func newRGWUserDataSource() datasource.DataSource {
	return &RGWUserDataSource{}
}

type RGWUserDataSource struct {
	client *restapi.Client
}

type RGWUserDataSourceModel struct {
	UserID      types.String `tfsdk:"user_id"`
	DisplayName types.String `tfsdk:"display_name"`
	Email       types.String `tfsdk:"email"`
	MaxBuckets  types.Int64  `tfsdk:"max_buckets"`
	System      types.Bool   `tfsdk:"system"`
	Suspended   types.Bool   `tfsdk:"suspended"`
	Tenant      types.String `tfsdk:"tenant"`
	Admin       types.Bool   `tfsdk:"admin"`
	Caps        types.Map    `tfsdk:"caps"`
	MfaIDs      types.List   `tfsdk:"mfa_ids"`
}

func (d *RGWUserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_user"
}

func (d *RGWUserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a Ceph RGW user.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"user_id": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The user identifier for this RGW user",
				Required:            true,
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
				MarkdownDescription: "The tenant this user belongs to (empty string for default tenant in multi-tenancy configurations)",
				Computed:            true,
			},
			"admin": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether this user has admin privileges (can only be set via radosgw-admin CLI)",
				Computed:            true,
			},
			"caps": dataSourceSchema.MapAttribute{
				MarkdownDescription: "Administrative capabilities of the user",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"mfa_ids": dataSourceSchema.ListAttribute{
				MarkdownDescription: "MFA device ids registered for the user",
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (d *RGWUserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RGWUserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RGWUserDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	userID := data.UserID.ValueString()
	user, err := d.client.RGWGetUser(ctx, userID)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get RGW user from Ceph API: %s", err),
		)
		return
	}

	if user.Tenant != "" {
		data.UserID = types.StringValue(user.Tenant + "$" + user.UserID)
	} else {
		data.UserID = types.StringValue(user.UserID)
	}
	data.DisplayName = types.StringValue(user.DisplayName)
	data.Email = types.StringValue(user.Email)
	data.MaxBuckets = types.Int64Value(int64(user.MaxBuckets))
	data.System = types.BoolValue(user.System)
	data.Suspended = types.BoolValue(user.Suspended == 1)
	data.Tenant = types.StringValue(user.Tenant)
	data.Admin = types.BoolValue(user.Admin)

	capsMap := make(map[string]string, len(user.Caps))
	for _, c := range user.Caps {
		capsMap[c.Type] = c.Perm
	}
	caps, diags := types.MapValueFrom(ctx, types.StringType, capsMap)
	resp.Diagnostics.Append(diags...)
	mfaIDs := user.MfaIDs
	if mfaIDs == nil {
		mfaIDs = []string{}
	}
	mfa, diags2 := types.ListValueFrom(ctx, types.StringType, mfaIDs)
	resp.Diagnostics.Append(diags2...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Caps = caps
	data.MfaIDs = mfa

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
