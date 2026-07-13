package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &RGWUserQuotaDataSource{}

func newRGWUserQuotaDataSource() datasource.DataSource {
	return &RGWUserQuotaDataSource{}
}

type RGWUserQuotaDataSource struct {
	client *restapi.Client
}

type RGWUserQuotaDataSourceModel struct {
	UID        types.String `tfsdk:"uid"`
	QuotaType  types.String `tfsdk:"quota_type"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	MaxSize    types.Int64  `tfsdk:"max_size"`
	MaxSizeKB  types.Int64  `tfsdk:"max_size_kb"`
	MaxObjects types.Int64  `tfsdk:"max_objects"`
}

func (d *RGWUserQuotaDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_user_quota"
}

func (d *RGWUserQuotaDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get a quota of a RadosGW user.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"uid": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The RGW user ID",
				Required:            true,
			},
			"quota_type": dataSourceSchema.StringAttribute{
				MarkdownDescription: "Which quota to read: `user` or `bucket`",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("user", "bucket"),
				},
			},
			"enabled": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the quota is enforced",
				Computed:            true,
			},
			"max_size": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum size in bytes. Negative means unlimited",
				Computed:            true,
			},
			"max_size_kb": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum size in kilobytes. -1 means unlimited",
				Computed:            true,
			},
			"max_objects": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum number of objects. -1 means unlimited",
				Computed:            true,
			},
		},
	}
}

func (d *RGWUserQuotaDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RGWUserQuotaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RGWUserQuotaDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	quotas, err := d.client.RGWGetUserQuota(ctx, data.UID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get quota for RGW user '%s' from Ceph API: %s", data.UID.ValueString(), err),
		)
		return
	}

	quota := rgwQuotaForType(quotas, data.QuotaType.ValueString())

	data.Enabled = types.BoolValue(quota.Enabled)
	data.MaxSize = types.Int64Value(quota.MaxSize)
	data.MaxObjects = types.Int64Value(quota.MaxObjects)
	if quota.MaxSize < 0 {
		data.MaxSizeKB = types.Int64Value(-1)
	} else {
		data.MaxSizeKB = types.Int64Value(quota.MaxSizeKB)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
