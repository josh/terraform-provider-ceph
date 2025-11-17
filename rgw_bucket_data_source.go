package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RGWBucketDataSource{}

func newRGWBucketDataSource() datasource.DataSource {
	return &RGWBucketDataSource{}
}

type RGWBucketDataSource struct {
	client *CephAPIClient
}

type RGWBucketDataSourceModel struct {
	Bucket        types.String `tfsdk:"bucket"`
	Zonegroup     types.String `tfsdk:"zonegroup"`
	PlacementRule types.String `tfsdk:"placement_rule"`
	ID            types.String `tfsdk:"id"`
	Owner         types.String `tfsdk:"owner"`
	CreationTime  types.String `tfsdk:"creation_time"`
	ACL           types.String `tfsdk:"acl"`
	Bid           types.String `tfsdk:"bid"`
}

func (d *RGWBucketDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_bucket"
}

func (d *RGWBucketDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a Ceph RGW bucket.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"bucket": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The bucket name",
				Required:            true,
			},
			"zonegroup": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The zonegroup this bucket belongs to",
				Computed:            true,
			},
			"placement_rule": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The placement rule for this bucket",
				Computed:            true,
			},
			"id": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The bucket ID",
				Computed:            true,
			},
			"owner": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The user ID of the bucket owner",
				Computed:            true,
			},
			"creation_time": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The creation timestamp of the bucket",
				Computed:            true,
			},
			"acl": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The Access Control List for this bucket",
				Computed:            true,
			},
			"bid": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The bucket ID (alternate field)",
				Computed:            true,
			},
		},
	}
}

func (d *RGWBucketDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *CephAPIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *RGWBucketDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RGWBucketDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	bucketName := data.Bucket.ValueString()
	bucket, err := d.client.RGWGetBucket(ctx, bucketName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get RGW bucket from Ceph API: %s", err),
		)
		return
	}

	data.Bucket = types.StringValue(bucket.Bucket)
	data.Zonegroup = types.StringValue(bucket.Zonegroup)
	data.PlacementRule = types.StringValue(bucket.PlacementRule)
	data.ID = types.StringValue(bucket.ID)
	data.Owner = types.StringValue(bucket.Owner)
	data.CreationTime = types.StringValue(bucket.CreationTime)
	data.ACL = types.StringValue(bucket.ACL)
	data.Bid = types.StringValue(bucket.Bid)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
