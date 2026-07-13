package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &RBDImageDataSource{}

func newRBDImageDataSource() datasource.DataSource {
	return &RBDImageDataSource{}
}

type RBDImageDataSource struct {
	client *restapi.Client
}

type RBDImageDataSourceModel struct {
	PoolName        types.String `tfsdk:"pool_name"`
	Namespace       types.String `tfsdk:"namespace"`
	Name            types.String `tfsdk:"name"`
	Size            types.Int64  `tfsdk:"size"`
	DataPool        types.String `tfsdk:"data_pool"`
	ID              types.String `tfsdk:"id"`
	BlockNamePrefix types.String `tfsdk:"block_name_prefix"`
	ObjectSize      types.Int64  `tfsdk:"object_size"`
	FeaturesName    types.Set    `tfsdk:"features_name"`
	Configuration   types.Map    `tfsdk:"configuration"`
	Metadata        types.Map    `tfsdk:"metadata"`
}

func (d *RBDImageDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rbd_image"
}

func (d *RBDImageDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about an RBD image.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"pool_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the pool holding the image.",
				Required:            true,
			},
			"namespace": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The RBD namespace holding the image.",
				Optional:            true,
			},
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the image.",
				Required:            true,
			},
			"size": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The size of the image in bytes.",
				Computed:            true,
			},
			"data_pool": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The erasure coded pool holding the image data.",
				Computed:            true,
			},
			"id": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The internal id of the image.",
				Computed:            true,
			},
			"block_name_prefix": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The prefix of the RADOS objects backing the image.",
				Computed:            true,
			},
			"object_size": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The object size of the image in bytes.",
				Computed:            true,
			},
			"features_name": dataSourceSchema.SetAttribute{
				MarkdownDescription: "The features enabled on the image.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"configuration": dataSourceSchema.MapAttribute{
				MarkdownDescription: "Image-level RBD configuration overrides.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"metadata": dataSourceSchema.MapAttribute{
				MarkdownDescription: "Arbitrary image metadata.",
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (d *RBDImageDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RBDImageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RBDImageDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	image, err := d.client.GetRBDImage(ctx, data.PoolName.ValueString(), data.Namespace.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get RBD image '%s' in pool '%s' from Ceph API: %s", data.Name.ValueString(), data.PoolName.ValueString(), err),
		)
		return
	}

	data.Size = types.Int64Value(image.Size)
	data.DataPool = types.StringPointerValue(image.DataPool)
	data.ID = types.StringValue(image.ID)
	data.BlockNamePrefix = types.StringValue(image.BlockNamePrefix)
	data.ObjectSize = types.Int64Value(image.ObjSize)

	featuresName, diags := types.SetValueFrom(ctx, types.StringType, image.FeaturesName)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.FeaturesName = featuresName

	imageConfig := map[string]string{}
	for _, option := range image.Configuration {
		if option.Source == 2 {
			imageConfig[option.Name] = option.Value
		}
	}
	configuration, diags2 := types.MapValueFrom(ctx, types.StringType, imageConfig)
	resp.Diagnostics.Append(diags2...)
	imageMetadata := image.Metadata
	if imageMetadata == nil {
		imageMetadata = map[string]string{}
	}
	metadata, diags3 := types.MapValueFrom(ctx, types.StringType, imageMetadata)
	resp.Diagnostics.Append(diags3...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Configuration = configuration
	data.Metadata = metadata

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
