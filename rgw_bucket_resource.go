package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &RGWBucketResource{}
	_ resource.ResourceWithImportState = &RGWBucketResource{}
)

func newRGWBucketResource() resource.Resource {
	return &RGWBucketResource{}
}

type RGWBucketResource struct {
	client *CephAPIClient
}

type RGWBucketResourceModel struct {
	Bucket        types.String `tfsdk:"bucket"`
	Owner         types.String `tfsdk:"owner"`
	Zonegroup     types.String `tfsdk:"zonegroup"`
	PlacementRule types.String `tfsdk:"placement_rule"`
	ID            types.String `tfsdk:"id"`
	CreationTime  types.String `tfsdk:"creation_time"`
	ACL           types.String `tfsdk:"acl"`
	Bid           types.String `tfsdk:"bid"`
}

func (r *RGWBucketResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_bucket"
}

func (r *RGWBucketResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource allows you to manage a Ceph RGW bucket.",
		Attributes: map[string]resourceSchema.Attribute{
			"bucket": resourceSchema.StringAttribute{
				MarkdownDescription: "The bucket name",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"owner": resourceSchema.StringAttribute{
				MarkdownDescription: "The user ID of the bucket owner",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"zonegroup": resourceSchema.StringAttribute{
				MarkdownDescription: "The zonegroup this bucket belongs to",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"placement_rule": resourceSchema.StringAttribute{
				MarkdownDescription: "The placement rule for this bucket",
				Computed:            true,
			},
			"id": resourceSchema.StringAttribute{
				MarkdownDescription: "The bucket ID",
				Computed:            true,
			},
			"creation_time": resourceSchema.StringAttribute{
				MarkdownDescription: "The creation timestamp of the bucket",
				Computed:            true,
			},
			"acl": resourceSchema.StringAttribute{
				MarkdownDescription: "The Access Control List for this bucket",
				Computed:            true,
			},
			"bid": resourceSchema.StringAttribute{
				MarkdownDescription: "The bucket ID (alternate field)",
				Computed:            true,
			},
		},
	}
}

func (r *RGWBucketResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *CephAPIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *RGWBucketResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RGWBucketResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createReq := CephAPIRGWBucketCreateRequest{
		Bucket: data.Bucket.ValueString(),
		UID:    data.Owner.ValueString(),
	}

	if !data.Zonegroup.IsNull() && !data.Zonegroup.IsUnknown() {
		zonegroup := data.Zonegroup.ValueString()
		createReq.Zonegroup = &zonegroup
	}

	_, err := r.client.RGWCreateBucket(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create RGW bucket: %s", err),
		)
		return
	}

	bucketName := data.Bucket.ValueString()
	bucket, err := r.client.RGWGetBucket(ctx, bucketName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW bucket after creation: %s", err),
		)
		return
	}

	updateModelFromAPIBucket(&data, bucket)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWBucketResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RGWBucketResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	bucketName := data.Bucket.ValueString()
	bucket, err := r.client.RGWGetBucket(ctx, bucketName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW bucket: %s", err),
		)
		return
	}

	updateModelFromAPIBucket(&data, bucket)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWBucketResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update Not Supported",
		"RGW buckets cannot be updated. All bucket attributes require replacement.",
	)
}

func (r *RGWBucketResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RGWBucketResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	bucketName := data.Bucket.ValueString()
	err := r.client.RGWDeleteBucket(ctx, bucketName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete RGW bucket: %s", err),
		)
		return
	}
}

func (r *RGWBucketResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	bucketName := req.ID

	bucket, err := r.client.RGWGetBucket(ctx, bucketName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW bucket during import: %s", err),
		)
		return
	}

	data := RGWBucketResourceModel{
		Bucket: types.StringValue(bucketName),
	}
	updateModelFromAPIBucket(&data, bucket)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func updateModelFromAPIBucket(data *RGWBucketResourceModel, bucket CephAPIRGWBucket) {
	data.Bucket = types.StringValue(bucket.Bucket)
	data.Owner = types.StringValue(bucket.Owner)
	data.Zonegroup = types.StringValue(bucket.Zonegroup)
	data.PlacementRule = types.StringValue(bucket.PlacementRule)
	data.ID = types.StringValue(bucket.ID)
	data.CreationTime = types.StringValue(bucket.CreationTime)
	data.ACL = types.StringValue(bucket.ACL)
	data.Bid = types.StringValue(bucket.Bid)
}
