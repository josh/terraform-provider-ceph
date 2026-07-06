package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &CephFSSubvolumeDataSource{}

func newCephFSSubvolumeDataSource() datasource.DataSource {
	return &CephFSSubvolumeDataSource{}
}

type CephFSSubvolumeDataSource struct {
	client *restapi.Client
}

type CephFSSubvolumeDataSourceModel struct {
	Name     types.String `tfsdk:"name"`
	VolName  types.String `tfsdk:"vol_name"`
	Size     types.Int64  `tfsdk:"size"`
	Path     types.String `tfsdk:"path"`
	DataPool types.String `tfsdk:"data_pool"`
	State    types.String `tfsdk:"state"`
}

func (d *CephFSSubvolumeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs_subvolume"
}

func (d *CephFSSubvolumeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a CephFS subvolume.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the subvolume.",
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
			"path": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The path of the subvolume within the filesystem.",
				Computed:            true,
			},
			"data_pool": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The data pool used by the subvolume.",
				Computed:            true,
			},
			"state": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The state of the subvolume.",
				Computed:            true,
			},
		},
	}
}

func (d *CephFSSubvolumeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CephFSSubvolumeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CephFSSubvolumeDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	info, err := d.client.CephFSSubvolumeInfo(ctx, data.VolName.ValueString(), data.Name.ValueString(), "")
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get CephFS subvolume '%s' in '%s' from Ceph API: %s", data.Name.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	data.Path = types.StringValue(info.Path)
	data.DataPool = types.StringValue(info.DataPool)
	data.State = types.StringValue(info.State)

	if quota, ok := info.BytesQuotaInt64(); ok && quota > 0 {
		data.Size = types.Int64Value(quota)
	} else {
		data.Size = types.Int64Null()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
