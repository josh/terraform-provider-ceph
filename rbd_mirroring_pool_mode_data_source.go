package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &RBDMirroringPoolModeDataSource{}

func newRBDMirroringPoolModeDataSource() datasource.DataSource {
	return &RBDMirroringPoolModeDataSource{}
}

type RBDMirroringPoolModeDataSource struct {
	client *restapi.Client
}

type RBDMirroringPoolModeDataSourceModel struct {
	PoolName types.String `tfsdk:"pool_name"`
	Mode     types.String `tfsdk:"mode"`
}

func (d *RBDMirroringPoolModeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rbd_mirroring_pool_mode"
}

func (d *RBDMirroringPoolModeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get the RBD mirroring mode of a pool.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"pool_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the pool",
				Required:            true,
			},
			"mode": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The mirroring mode of the pool: `disabled`, `image`, `pool` or `unknown`",
				Computed:            true,
			},
		},
	}
}

func (d *RBDMirroringPoolModeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RBDMirroringPoolModeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RBDMirroringPoolModeDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	mode, err := d.client.GetRBDMirroringPoolMode(ctx, data.PoolName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get mirroring mode of pool '%s' from Ceph API: %s", data.PoolName.ValueString(), err),
		)
		return
	}

	data.Mode = types.StringValue(mode.MirrorMode)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
