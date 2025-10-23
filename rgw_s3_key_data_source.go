package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RGWS3KeyDataSource{}

func newRGWS3KeyDataSource() datasource.DataSource {
	return &RGWS3KeyDataSource{}
}

type RGWS3KeyDataSource struct {
	client *CephAPIClient
}

type RGWS3KeyDataSourceModel struct {
	UID       types.String `tfsdk:"uid"`
	AccessKey types.String `tfsdk:"access_key"`
	SecretKey types.String `tfsdk:"secret_key"`
	User      types.String `tfsdk:"user"`
	Active    types.Bool   `tfsdk:"active"`
}

func (d *RGWS3KeyDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_s3_key"
}

func (d *RGWS3KeyDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a Ceph RGW S3 access key.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"uid": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The user ID that owns the S3 key",
				Required:            true,
			},
			"access_key": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The S3 access key ID to look up",
				Required:            true,
			},
			"secret_key": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The S3 secret key",
				Computed:            true,
				Sensitive:           true,
			},
			"user": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The user associated with the key",
				Computed:            true,
			},
			"active": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the key is active",
				Computed:            true,
			},
		},
	}
}

func (d *RGWS3KeyDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RGWS3KeyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RGWS3KeyDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	uid := data.UID.ValueString()
	accessKey := data.AccessKey.ValueString()

	user, err := d.client.RGWGetUser(ctx, uid)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get RGW user from Ceph API: %s", err),
		)
		return
	}

	var foundKey *CephAPIRGWUserKey
	for i := range user.Keys {
		if user.Keys[i].AccessKey == accessKey {
			foundKey = &user.Keys[i]
			break
		}
	}

	if foundKey == nil {
		resp.Diagnostics.AddError(
			"Key Not Found",
			fmt.Sprintf("S3 access key %s not found for user %s", accessKey, uid),
		)
		return
	}

	data.SecretKey = types.StringValue(foundKey.SecretKey)
	data.User = types.StringValue(foundKey.User)
	data.Active = types.BoolValue(foundKey.Active)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
