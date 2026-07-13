package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &RGWBucketResource{}
	_ resource.ResourceWithImportState = &RGWBucketResource{}
)

func newRGWBucketResource() resource.Resource {
	return &RGWBucketResource{}
}

type RGWBucketResource struct {
	client *restapi.Client
}

type RGWBucketResourceModel struct {
	Bucket          types.String         `tfsdk:"bucket"`
	Owner           types.String         `tfsdk:"owner"`
	VersioningState types.String         `tfsdk:"versioning_state"`
	Tags            types.Map            `tfsdk:"tags"`
	BucketPolicy    jsontypes.Normalized `tfsdk:"bucket_policy"`
	Zonegroup       types.String         `tfsdk:"zonegroup"`
	PlacementRule   types.String         `tfsdk:"placement_rule"`
	ID              types.String         `tfsdk:"id"`
	CreationTime    types.String         `tfsdk:"creation_time"`
	ACL             types.String         `tfsdk:"acl"`
	Bid             types.String         `tfsdk:"bid"`
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
				MarkdownDescription: "The user ID of the bucket owner. Changing re-links the bucket to the new owner in place.",
				Required:            true,
			},
			"versioning_state": resourceSchema.StringAttribute{
				MarkdownDescription: "The S3 versioning state of the bucket: `Enabled` or `Suspended`. A never-versioned bucket reports `Suspended`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("Enabled", "Suspended"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tags": resourceSchema.MapAttribute{
				MarkdownDescription: "The S3 tags of the bucket. Set to an empty map to clear all tags.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket_policy": resourceSchema.StringAttribute{
				MarkdownDescription: "The S3 bucket policy as a JSON document. The API has no way to delete a policy; to neutralize one, set a policy with an empty statement list.",
				Optional:            true,
				Computed:            true,
				CustomType:          jsontypes.NormalizedType{},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"zonegroup": resourceSchema.StringAttribute{
				MarkdownDescription: "The zonegroup this bucket belongs to",
				Computed:            true,
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

	client, ok := req.ProviderData.(*restapi.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *restapi.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
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

	createReq := restapi.RGWBucketCreateRequest{
		Bucket: data.Bucket.ValueString(),
		UID:    data.Owner.ValueString(),
	}

	if tagsXML, diags := tagsXMLFromModel(ctx, data.Tags); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	} else if tagsXML != nil {
		createReq.Tags = tagsXML
	}
	if !data.BucketPolicy.IsNull() && !data.BucketPolicy.IsUnknown() {
		createReq.BucketPolicy = data.BucketPolicy.ValueStringPointer()
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
	bucket, err := r.client.RGWGetBucketWithRetry(ctx, bucketName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW bucket after creation: %s", err),
		)
		return
	}

	// Versioning cannot be set at creation time.
	if data.VersioningState.ValueString() == "Enabled" && bucket.Versioning != "Enabled" {
		versioning := "Enabled"
		err = r.client.RGWUpdateBucket(ctx, bucketName, restapi.RGWBucketUpdateRequest{
			BucketID:        bucket.ID,
			UID:             data.Owner.ValueString(),
			VersioningState: &versioning,
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to enable versioning on RGW bucket '%s': %s", bucketName, err),
			)
			return
		}
		bucket, err = r.client.RGWGetBucketWithRetry(ctx, bucketName)
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to read RGW bucket after enabling versioning: %s", err),
			)
			return
		}
	}

	resp.Diagnostics.Append(updateModelFromAPIBucket(ctx, &data, bucket)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWBucketResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RGWBucketResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	bucketName := data.Bucket.ValueString()
	bucket, err := r.client.RGWGetBucketWithRetry(ctx, bucketName)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW bucket: %s", err),
		)
		return
	}

	resp.Diagnostics.Append(updateModelFromAPIBucket(ctx, &data, bucket)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWBucketResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RGWBucketResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bucketName := data.Bucket.ValueString()

	current, err := r.client.RGWGetBucketWithRetry(ctx, bucketName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW bucket before update: %s", err),
		)
		return
	}

	// The set endpoint always needs bucket_id and uid, and deletes the
	// lifecycle and encryption configuration when they are omitted, so
	// the current values are carried through.
	updateReq := restapi.RGWBucketUpdateRequest{
		BucketID: current.ID,
		UID:      data.Owner.ValueString(),
	}

	if !data.VersioningState.IsNull() && !data.VersioningState.IsUnknown() {
		updateReq.VersioningState = data.VersioningState.ValueStringPointer()
	}
	if tagsXML, diags := tagsXMLFromModel(ctx, data.Tags); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	} else if tagsXML != nil {
		updateReq.Tags = tagsXML
	}
	if !data.BucketPolicy.IsNull() && !data.BucketPolicy.IsUnknown() {
		updateReq.BucketPolicy = data.BucketPolicy.ValueStringPointer()
	}
	if strings.Contains(string(current.Encryption), "Enabled") {
		encryption := "true"
		updateReq.EncryptionState = &encryption
	}
	if lifecycle := string(current.Lifecycle); lifecycle != "" && lifecycle != "null" && lifecycle != "{}" {
		updateReq.Lifecycle = &lifecycle
	}

	err = r.client.RGWUpdateBucket(ctx, bucketName, updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update RGW bucket '%s': %s", bucketName, err),
		)
		return
	}

	bucket, err := r.client.RGWGetBucketWithRetry(ctx, bucketName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW bucket after update: %s", err),
		)
		return
	}

	resp.Diagnostics.Append(updateModelFromAPIBucket(ctx, &data, bucket)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
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
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete RGW bucket: %s", err),
		)
		return
	}
}

func (r *RGWBucketResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("bucket"), req, resp)
}

func updateModelFromAPIBucket(ctx context.Context, data *RGWBucketResourceModel, bucket *restapi.RGWBucket) diag.Diagnostics {
	var diags diag.Diagnostics

	data.Bucket = types.StringValue(bucket.Bucket)
	data.Owner = types.StringValue(bucket.Owner)
	data.Zonegroup = types.StringValue(bucket.Zonegroup)
	data.PlacementRule = types.StringValue(bucket.PlacementRule)
	data.ID = types.StringValue(bucket.ID)
	data.CreationTime = types.StringValue(bucket.CreationTime)
	data.ACL = types.StringValue(bucket.ACL)
	data.Bid = types.StringValue(bucket.Bid)
	data.VersioningState = types.StringValue(bucket.Versioning)

	tagset := bucket.Tagset
	if tagset == nil {
		tagset = map[string]string{}
	}
	tags, d := types.MapValueFrom(ctx, types.StringType, tagset)
	diags.Append(d...)
	data.Tags = tags

	if policy := string(bucket.BucketPolicy); policy != "" && policy != "null" {
		data.BucketPolicy = jsontypes.NewNormalizedValue(policy)
	} else {
		data.BucketPolicy = jsontypes.NewNormalizedNull()
	}

	return diags
}

// tagsXMLFromModel renders a planned tags map into the S3 tagging XML
// document, returning nil when the attribute is unset.
func tagsXMLFromModel(ctx context.Context, tags types.Map) (*string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if tags.IsNull() || tags.IsUnknown() {
		return nil, diags
	}

	var tagsMap map[string]string
	diags.Append(tags.ElementsAs(ctx, &tagsMap, false)...)
	if diags.HasError() {
		return nil, diags
	}

	xml := restapi.TagsMapToXML(tagsMap)
	return &xml, diags
}
