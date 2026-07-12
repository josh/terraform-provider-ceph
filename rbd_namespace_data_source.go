package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &RBDNamespaceDataSource{}

func newRBDNamespaceDataSource() datasource.DataSource {
	return &RBDNamespaceDataSource{}
}

type RBDNamespaceDataSource struct {
	client *restapi.Client
}

type RBDNamespaceDataSourceModel struct {
	PoolName  types.String `tfsdk:"pool_name"`
	Name      types.String `tfsdk:"name"`
	NumImages types.Int64  `tfsdk:"num_images"`
}

func (d *RBDNamespaceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rbd_namespace"
}

func (d *RBDNamespaceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about an RBD namespace.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"pool_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the pool containing the namespace",
				Required:            true,
			},
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the RBD namespace",
				Required:            true,
			},
			"num_images": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The number of RBD images in the namespace",
				Computed:            true,
			},
		},
	}
}

func (d *RBDNamespaceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RBDNamespaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RBDNamespaceDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	namespace, err := d.client.GetRBDNamespace(ctx, data.PoolName.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get RBD namespace '%s' in pool '%s' from Ceph API: %s", data.Name.ValueString(), data.PoolName.ValueString(), err),
		)
		return
	}

	data.NumImages = types.Int64Value(namespace.NumImages)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
