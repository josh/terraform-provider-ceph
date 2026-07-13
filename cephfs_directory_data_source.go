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

var _ datasource.DataSource = &CephFSDirectoryDataSource{}

func newCephFSDirectoryDataSource() datasource.DataSource {
	return &CephFSDirectoryDataSource{}
}

type CephFSDirectoryDataSource struct {
	client *restapi.Client
}

type CephFSDirectoryDataSourceModel struct {
	VolName types.String `tfsdk:"vol_name"`
	Path    types.String `tfsdk:"path"`
}

func (d *CephFSDirectoryDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs_directory"
}

func (d *CephFSDirectoryDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source verifies that a directory exists in a CephFS filesystem.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"vol_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem",
				Required:            true,
			},
			"path": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The absolute directory path within the filesystem",
				Required:            true,
				Validators: []validator.String{
					cephFSDirectoryPath(),
				},
			},
		},
	}
}

func (d *CephFSDirectoryDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CephFSDirectoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CephFSDirectoryDataSourceModel

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

	_, err = d.client.CephFSGetDirectory(ctx, fs.ID, data.Path.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.Diagnostics.AddError(
				"Directory Not Found",
				fmt.Sprintf("Directory '%s' does not exist in '%s'", data.Path.ValueString(), data.VolName.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read directory '%s' in '%s': %s", data.Path.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
