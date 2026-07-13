package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &CephFSSnapshotDataSource{}

func newCephFSSnapshotDataSource() datasource.DataSource {
	return &CephFSSnapshotDataSource{}
}

type CephFSSnapshotDataSource struct {
	client *restapi.Client
}

type CephFSSnapshotDataSourceModel struct {
	VolName types.String `tfsdk:"vol_name"`
	Path    types.String `tfsdk:"path"`
	Name    types.String `tfsdk:"name"`
	Created types.String `tfsdk:"created"`
}

func (d *CephFSSnapshotDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs_snapshot"
}

func (d *CephFSSnapshotDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a snapshot of a CephFS directory.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"vol_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem",
				Required:            true,
			},
			"path": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The directory path",
				Required:            true,
				Validators: []validator.String{
					cephFSDirectoryPath(),
				},
			},
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the snapshot",
				Required:            true,
			},
			"created": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The snapshot creation timestamp",
				Computed:            true,
			},
		},
	}
}

func (d *CephFSSnapshotDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CephFSSnapshotDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CephFSSnapshotDataSourceModel

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

	snap, err := d.client.CephFSGetSnapshot(ctx, fs.ID, data.Path.ValueString(), data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.Diagnostics.AddError(
				"Snapshot Not Found",
				fmt.Sprintf("Snapshot '%s' of '%s' does not exist in '%s'", data.Name.ValueString(), data.Path.ValueString(), data.VolName.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read snapshot '%s' of '%s' in '%s': %s", data.Name.ValueString(), data.Path.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	data.Created = types.StringValue(snap.Created)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
