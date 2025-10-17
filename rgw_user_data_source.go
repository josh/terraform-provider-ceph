package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	MaxBuckets  types.Int64  `tfsdk:"max_buckets"`
	System      types.Bool   `tfsdk:"system"`
	Admin       types.Bool   `tfsdk:"admin"`
	Keys        types.List   `tfsdk:"keys"`
}

type RGWUserKeyModel struct {
	User      types.String `tfsdk:"user"`
	AccessKey types.String `tfsdk:"access_key"`
	SecretKey types.String `tfsdk:"secret_key"`
	Active    types.Bool   `tfsdk:"active"`
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
			"max_buckets": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "Maximum number of buckets the user can own",
				Computed:            true,
			},
			"system": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether this is a system user",
				Computed:            true,
			},
			"admin": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether this user has admin privileges",
				Computed:            true,
			},
			"keys": dataSourceSchema.ListNestedAttribute{
				MarkdownDescription: "S3/Swift keys for the user",
				Computed:            true,
				NestedObject: dataSourceSchema.NestedAttributeObject{
					Attributes: map[string]dataSourceSchema.Attribute{
						"user": dataSourceSchema.StringAttribute{
							MarkdownDescription: "The user ID associated with the key",
							Computed:            true,
						},
						"access_key": dataSourceSchema.StringAttribute{
							MarkdownDescription: "The access key",
							Computed:            true,
							Sensitive:           true,
						},
						"secret_key": dataSourceSchema.StringAttribute{
							MarkdownDescription: "The secret key",
							Computed:            true,
							Sensitive:           true,
						},
						"active": dataSourceSchema.BoolAttribute{
							MarkdownDescription: "Whether the key is active",
							Computed:            true,
						},
					},
				},
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
	data.MaxBuckets = types.Int64Value(int64(user.MaxBuckets))
	data.System = types.BoolValue(user.System)
	data.Admin = types.BoolValue(user.Admin)

	keysList := make([]RGWUserKeyModel, len(user.Keys))
	for i, key := range user.Keys {
		keysList[i] = RGWUserKeyModel{
			User:      types.StringValue(key.User),
			AccessKey: types.StringValue(key.AccessKey),
			SecretKey: types.StringValue(key.SecretKey),
			Active:    types.BoolValue(key.Active),
		}
	}

	keysListValue, diags := types.ListValueFrom(ctx, types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"user":       types.StringType,
			"access_key": types.StringType,
			"secret_key": types.StringType,
			"active":     types.BoolType,
		},
	}, keysList)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Keys = keysListValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
