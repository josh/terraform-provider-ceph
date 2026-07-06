package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &CephFSSubvolumeGroupDataSource{}

func newCephFSSubvolumeGroupDataSource() datasource.DataSource {
	return &CephFSSubvolumeGroupDataSource{}
}

type CephFSSubvolumeGroupDataSource struct {
	client *restapi.Client
}

type CephFSSubvolumeGroupDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	VolName types.String `tfsdk:"vol_name"`
	Size    types.Int64  `tfsdk:"size"`
}

func (d *CephFSSubvolumeGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs_subvolume_group"
}

func (d *CephFSSubvolumeGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a CephFS subvolume group.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the subvolume group.",
				Required:            true,
			},
			"vol_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem.",
				Required:            true,
			},
			"size": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The quota size in bytes.",
				Computed:            true,
			},
		},
	}
}

func (d *CephFSSubvolumeGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CephFSSubvolumeGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CephFSSubvolumeGroupDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	info, err := d.client.CephFSSubvolumeGroupInfo(ctx, data.VolName.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get CephFS subvolume group '%s' in '%s' from Ceph API: %s", data.Name.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	if quota, ok := info.BytesQuotaInt64(); ok && quota > 0 {
		data.Size = types.Int64Value(quota)
	} else {
		data.Size = types.Int64Null()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
