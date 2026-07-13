package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &RGWUserQuotaResource{}
	_ resource.ResourceWithImportState = &RGWUserQuotaResource{}
)

func newRGWUserQuotaResource() resource.Resource {
	return &RGWUserQuotaResource{}
}

type RGWUserQuotaResource struct {
	client *restapi.Client
}

type RGWUserQuotaResourceModel struct {
	UID        types.String `tfsdk:"uid"`
	QuotaType  types.String `tfsdk:"quota_type"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	MaxSizeKB  types.Int64  `tfsdk:"max_size_kb"`
	MaxObjects types.Int64  `tfsdk:"max_objects"`
}

func (r *RGWUserQuotaResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_user_quota"
}

func (r *RGWUserQuotaResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a quota of a RadosGW user. Every user always has both a user and a bucket quota configuration; creating this resource sets one of them and destroying it resets that quota to disabled and unlimited.",
		Attributes: map[string]resourceSchema.Attribute{
			"uid": resourceSchema.StringAttribute{
				MarkdownDescription: "The RGW user ID, including an optional tenant prefix in the `tenant$uid` form.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"quota_type": resourceSchema.StringAttribute{
				MarkdownDescription: "Which quota to manage: `user` limits the user's total usage, `bucket` limits each of the user's buckets.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("user", "bucket"),
				},
			},
			"enabled": resourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the quota is enforced. Defaults to false.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"max_size_kb": resourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum size in kilobytes. -1 means unlimited.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(-1),
				Validators: []validator.Int64{
					int64validator.AtLeast(-1),
				},
			},
			"max_objects": resourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum number of objects. -1 means unlimited.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(-1),
				Validators: []validator.Int64{
					int64validator.AtLeast(-1),
				},
			},
		},
	}
}

func (r *RGWUserQuotaResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func rgwQuotaForType(quotas *restapi.RGWUserQuotas, quotaType string) restapi.RGWQuotaInfo {
	if quotaType == "bucket" {
		return quotas.BucketQuota
	}
	return quotas.UserQuota
}

func (r *RGWUserQuotaResource) updateModelFromAPI(data *RGWUserQuotaResourceModel, quota restapi.RGWQuotaInfo) {
	data.Enabled = types.BoolValue(quota.Enabled)
	data.MaxObjects = types.Int64Value(quota.MaxObjects)
	// Unlimited sizes come back in several encodings: a fresh user has
	// max_size -1, while the admin op stores max_size_kb*1024 without
	// clamping, so -1 becomes -1024, and max_size_kb dumps as a garbage
	// unsigned value for any of them. Any negative max_size means
	// unlimited, and max_size_kb is only trustworthy for positive sizes.
	if quota.MaxSize < 0 {
		data.MaxSizeKB = types.Int64Value(-1)
	} else {
		data.MaxSizeKB = types.Int64Value(quota.MaxSizeKB)
	}
}

func (r *RGWUserQuotaResource) setAndRead(ctx context.Context, data *RGWUserQuotaResourceModel) error {
	uid := data.UID.ValueString()
	quotaType := data.QuotaType.ValueString()

	err := r.client.RGWSetUserQuota(ctx, uid, quotaType, data.Enabled.ValueBool(), data.MaxSizeKB.ValueInt64(), data.MaxObjects.ValueInt64())
	if err != nil {
		return fmt.Errorf("unable to set %s quota for RGW user '%s': %w", quotaType, uid, err)
	}

	quotas, err := r.client.RGWGetUserQuota(ctx, uid)
	if err != nil {
		return fmt.Errorf("unable to read quota for RGW user '%s' after setting it: %w", uid, err)
	}

	r.updateModelFromAPI(data, rgwQuotaForType(quotas, quotaType))
	return nil
}

func (r *RGWUserQuotaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RGWUserQuotaResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setAndRead(ctx, &data); err != nil {
		resp.Diagnostics.AddError("API Request Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWUserQuotaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RGWUserQuotaResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	quotas, err := r.client.RGWGetUserQuota(ctx, data.UID.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read quota for RGW user '%s': %s", data.UID.ValueString(), err),
		)
		return
	}

	r.updateModelFromAPI(&data, rgwQuotaForType(quotas, data.QuotaType.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWUserQuotaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RGWUserQuotaResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setAndRead(ctx, &data); err != nil {
		resp.Diagnostics.AddError("API Request Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWUserQuotaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RGWUserQuotaResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.RGWSetUserQuota(ctx, data.UID.ValueString(), data.QuotaType.ValueString(), false, -1, -1)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to reset %s quota for RGW user '%s': %s", data.QuotaType.ValueString(), data.UID.ValueString(), err),
		)
		return
	}
}

func (r *RGWUserQuotaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The uid may contain ':', so the quota type is bounded by the last
	// colon.
	last := strings.LastIndex(req.ID, ":")
	quotaType := ""
	if last > 0 {
		quotaType = req.ID[last+1:]
	}
	if quotaType != "user" && quotaType != "bucket" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: uid:user or uid:bucket, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uid"), req.ID[:last])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("quota_type"), quotaType)...)
}
