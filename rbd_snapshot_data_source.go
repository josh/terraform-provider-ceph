package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &RBDSnapshotDataSource{}

func newRBDSnapshotDataSource() datasource.DataSource {
	return &RBDSnapshotDataSource{}
}

type RBDSnapshotDataSource struct {
	client *restapi.Client
}

type RBDSnapshotDataSourceModel struct {
	PoolName    types.String `tfsdk:"pool_name"`
	ImageName   types.String `tfsdk:"image_name"`
	Name        types.String `tfsdk:"name"`
	IsProtected types.Bool   `tfsdk:"is_protected"`
	Size        types.Int64  `tfsdk:"size"`
	Timestamp   types.String `tfsdk:"timestamp"`
}

func (d *RBDSnapshotDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rbd_snapshot"
}

func (d *RBDSnapshotDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about an RBD image snapshot.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"pool_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the pool holding the image",
				Required:            true,
			},
			"image_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the image",
				Required:            true,
			},
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the snapshot",
				Required:            true,
			},
			"is_protected": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the snapshot is protected from deletion",
				Computed:            true,
			},
			"size": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The size of the image at snapshot time in bytes",
				Computed:            true,
			},
			"timestamp": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The snapshot creation timestamp",
				Computed:            true,
			},
		},
	}
}

func (d *RBDSnapshotDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RBDSnapshotDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RBDSnapshotDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	snap, err := d.client.GetRBDSnapshot(ctx, data.PoolName.ValueString(), data.ImageName.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get snapshot '%s' of RBD image '%s/%s' from Ceph API: %s", data.Name.ValueString(), data.PoolName.ValueString(), data.ImageName.ValueString(), err),
		)
		return
	}

	data.IsProtected = types.BoolValue(snap.IsProtected != nil && *snap.IsProtected)
	data.Size = types.Int64Value(snap.Size)
	data.Timestamp = types.StringValue(snap.Timestamp)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
