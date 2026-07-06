package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &CephFSDataSource{}

func newCephFSDataSource() datasource.DataSource {
	return &CephFSDataSource{}
}

type CephFSDataSource struct {
	client *restapi.Client
}

type CephFSDataSourceModel struct {
	Name           types.String `tfsdk:"name"`
	ID             types.Int64  `tfsdk:"id"`
	MetadataPoolID types.Int64  `tfsdk:"metadata_pool_id"`
	DataPoolIDs    types.List   `tfsdk:"data_pool_ids"`
}

func (d *CephFSDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs"
}

func (d *CephFSDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a CephFS filesystem.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem.",
				Required:            true,
			},
			"id": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The ID of the CephFS filesystem.",
				Computed:            true,
			},
			"metadata_pool_id": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The ID of the metadata pool.",
				Computed:            true,
			},
			"data_pool_ids": dataSourceSchema.ListAttribute{
				MarkdownDescription: "The IDs of the data pools.",
				Computed:            true,
				ElementType:         types.Int64Type,
			},
		},
	}
}

func (d *CephFSDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CephFSDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CephFSDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	fs, err := d.client.CephFSGetByName(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get CephFS filesystem '%s' from Ceph API: %s", data.Name.ValueString(), err),
		)
		return
	}

	data.ID = types.Int64Value(int64(fs.ID))
	data.MetadataPoolID = types.Int64Value(int64(fs.MetadataPoolID))

	if len(fs.DataPoolIDs) > 0 {
		poolIDs := make([]int64, len(fs.DataPoolIDs))
		for i, id := range fs.DataPoolIDs {
			poolIDs[i] = int64(id)
		}
		dataPoolIDs, diags := types.ListValueFrom(ctx, types.Int64Type, poolIDs)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.DataPoolIDs = dataPoolIDs
	} else {
		data.DataPoolIDs = types.ListNull(types.Int64Type)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
