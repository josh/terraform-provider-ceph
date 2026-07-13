package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &CephFSSubvolumeSnapshotDataSource{}

func newCephFSSubvolumeSnapshotDataSource() datasource.DataSource {
	return &CephFSSubvolumeSnapshotDataSource{}
}

type CephFSSubvolumeSnapshotDataSource struct {
	client *restapi.Client
}

type CephFSSubvolumeSnapshotDataSourceModel struct {
	VolName          types.String `tfsdk:"vol_name"`
	GroupName        types.String `tfsdk:"group_name"`
	SubvolName       types.String `tfsdk:"subvol_name"`
	Name             types.String `tfsdk:"name"`
	CreatedAt        types.String `tfsdk:"created_at"`
	DataPool         types.String `tfsdk:"data_pool"`
	HasPendingClones types.Bool   `tfsdk:"has_pending_clones"`
}

func (d *CephFSSubvolumeSnapshotDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs_subvolume_snapshot"
}

func (d *CephFSSubvolumeSnapshotDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a CephFS subvolume snapshot.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"vol_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem.",
				Required:            true,
			},
			"group_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The subvolume group holding the subvolume.",
				Optional:            true,
			},
			"subvol_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the subvolume.",
				Required:            true,
			},
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the snapshot.",
				Required:            true,
			},
			"created_at": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The snapshot creation timestamp.",
				Computed:            true,
			},
			"data_pool": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The data pool backing the snapshot.",
				Computed:            true,
			},
			"has_pending_clones": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the snapshot has clone operations in progress.",
				Computed:            true,
			},
		},
	}
}

func (d *CephFSSubvolumeSnapshotDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CephFSSubvolumeSnapshotDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CephFSSubvolumeSnapshotDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	info, err := d.client.CephFSSubvolumeSnapshotInfo(ctx, data.VolName.ValueString(), data.SubvolName.ValueString(), data.Name.ValueString(), data.GroupName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get snapshot '%s' of subvolume '%s' in '%s' from Ceph API: %s", data.Name.ValueString(), data.SubvolName.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	data.CreatedAt = types.StringValue(info.CreatedAt)
	data.DataPool = types.StringValue(info.DataPool)
	data.HasPendingClones = types.BoolValue(info.HasPendingClones == "yes")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
