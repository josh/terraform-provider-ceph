package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &CephFSQuotaDataSource{}

func newCephFSQuotaDataSource() datasource.DataSource {
	return &CephFSQuotaDataSource{}
}

type CephFSQuotaDataSource struct {
	client *restapi.Client
}

type CephFSQuotaDataSourceModel struct {
	VolName  types.String `tfsdk:"vol_name"`
	Path     types.String `tfsdk:"path"`
	MaxBytes types.Int64  `tfsdk:"max_bytes"`
	MaxFiles types.Int64  `tfsdk:"max_files"`
}

func (d *CephFSQuotaDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs_quota"
}

func (d *CephFSQuotaDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get the quota of a CephFS directory.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"vol_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem",
				Required:            true,
			},
			"path": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The directory path within the filesystem",
				Required:            true,
			},
			"max_bytes": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum number of bytes allowed under the path. 0 means unlimited",
				Computed:            true,
			},
			"max_files": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum number of files allowed under the path. 0 means unlimited",
				Computed:            true,
			},
		},
	}
}

func (d *CephFSQuotaDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CephFSQuotaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CephFSQuotaDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	fs, err := d.client.CephFSGetByName(ctx, data.VolName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to look up CephFS filesystem '%s': %s", data.VolName.ValueString(), err),
		)
		return
	}

	quota, err := d.client.GetCephFSQuota(ctx, fs.ID, data.Path.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get quota on '%s' in '%s' from Ceph API: %s", data.Path.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	data.MaxBytes = types.Int64Value(quota.MaxBytes)
	data.MaxFiles = types.Int64Value(quota.MaxFiles)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
