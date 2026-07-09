package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                     = &ErasureCodeProfileResource{}
	_ resource.ResourceWithImportState      = &ErasureCodeProfileResource{}
	_ resource.ResourceWithConfigValidators = &ErasureCodeProfileResource{}
	_ resource.ResourceWithModifyPlan       = &ErasureCodeProfileResource{}
)

func newErasureCodeProfileResource() resource.Resource {
	return &ErasureCodeProfileResource{}
}

type ErasureCodeProfileResource struct {
	client *restapi.Client
}

type ErasureCodeProfileResourceModel struct {
	Name                      types.String `tfsdk:"name"`
	K                         types.Int64  `tfsdk:"k"`
	M                         types.Int64  `tfsdk:"m"`
	Plugin                    types.String `tfsdk:"plugin"`
	CrushFailureDomain        types.String `tfsdk:"crush_failure_domain"`
	CrushNumFailureDomains    types.Int64  `tfsdk:"crush_num_failure_domains"`
	CrushOSDsPerFailureDomain types.Int64  `tfsdk:"crush_osds_per_failure_domain"`
	Technique                 types.String `tfsdk:"technique"`
	CrushRoot                 types.String `tfsdk:"crush_root"`
	CrushDeviceClass          types.String `tfsdk:"crush_device_class"`
	Directory                 types.String `tfsdk:"directory"`
}

type erasureCodeKMValidator struct{}

func (v erasureCodeKMValidator) Description(ctx context.Context) string {
	return "validates k and m values for erasure code profile"
}

func (v erasureCodeKMValidator) MarkdownDescription(ctx context.Context) string {
	return "Validates k and m values meet Ceph requirements: k+m <= 255 and m < k is recommended."
}

func (v erasureCodeKMValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config ErasureCodeProfileResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.K.IsUnknown() || config.M.IsUnknown() {
		return
	}

	if config.K.IsNull() || config.M.IsNull() {
		return
	}

	k := config.K.ValueInt64()
	m := config.M.ValueInt64()

	if k+m > 255 {
		resp.Diagnostics.Append(diag.NewErrorDiagnostic(
			"Invalid Erasure Code Configuration",
			fmt.Sprintf("The sum of k and m must not exceed 255. Got k=%d, m=%d, sum=%d.", k, m, k+m),
		))
	}

	if m >= k {
		resp.Diagnostics.Append(diag.NewWarningDiagnostic(
			"Unusual Erasure Code Configuration",
			fmt.Sprintf("The coding chunks (m=%d) should typically be less than data chunks (k=%d) for efficiency. This configuration is valid but may not provide optimal storage efficiency.", m, k),
		))
	}
}

func (r *ErasureCodeProfileResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_erasure_code_profile"
}

type erasureCodeFailureDomainValidator struct{}

func (v erasureCodeFailureDomainValidator) Description(ctx context.Context) string {
	return "validates crush_num_failure_domains is set when crush_osds_per_failure_domain is specified"
}

func (v erasureCodeFailureDomainValidator) MarkdownDescription(ctx context.Context) string {
	return "Validates that `crush_num_failure_domains` is >= 1 when `crush_osds_per_failure_domain` is specified."
}

func (v erasureCodeFailureDomainValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config ErasureCodeProfileResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.CrushOSDsPerFailureDomain.IsUnknown() || config.CrushOSDsPerFailureDomain.IsNull() {
		return
	}

	if config.CrushNumFailureDomains.IsUnknown() || config.CrushNumFailureDomains.IsNull() {
		resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
			path.Root("crush_num_failure_domains"),
			"Missing Required Attribute",
			"crush_num_failure_domains must be specified and >= 1 when crush_osds_per_failure_domain is set.",
		))
		return
	}

	if config.CrushNumFailureDomains.ValueInt64() < 1 {
		resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
			path.Root("crush_num_failure_domains"),
			"Invalid Erasure Code Configuration",
			fmt.Sprintf(
				"crush_num_failure_domains must be >= 1 when crush_osds_per_failure_domain is specified. Got crush_num_failure_domains=%d.",
				config.CrushNumFailureDomains.ValueInt64(),
			),
		))
	}
}

func (r *ErasureCodeProfileResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		erasureCodeKMValidator{},
		erasureCodeFailureDomainValidator{},
	}
}

func (r *ErasureCodeProfileResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a Ceph erasure code profile. Erasure code profiles are immutable in Ceph, so any changes to the profile's attributes will trigger resource replacement.",
		Attributes: map[string]resourceSchema.Attribute{
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the erasure code profile. This is the unique identifier for the profile.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"k": resourceSchema.Int64Attribute{
				MarkdownDescription: "Number of data chunks. Must be at least 2. Defaults to 2 if not specified.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(2),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"m": resourceSchema.Int64Attribute{
				MarkdownDescription: "Number of coding chunks (parity). Must be at least 1. Defaults to 1 if not specified.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"plugin": resourceSchema.StringAttribute{
				MarkdownDescription: "The erasure code plugin to use (e.g., 'jerasure', 'isa', 'lrc', 'shec', 'clay'). Defaults to 'jerasure' if not specified.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"crush_failure_domain": resourceSchema.StringAttribute{
				MarkdownDescription: "The CRUSH failure domain for placement (e.g., 'host', 'rack', 'osd'). Determines how chunks are distributed across the cluster. Defaults to 'host' if not specified.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"crush_num_failure_domains": resourceSchema.Int64Attribute{
				MarkdownDescription: "The number of failure domains across which the chunks should be distributed. Used with 'crush_osds_per_failure_domain' for fine-grained placement control.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"crush_osds_per_failure_domain": resourceSchema.Int64Attribute{
				MarkdownDescription: "The number of OSDs to use per failure domain. Used with 'crush_num_failure_domains' for fine-grained placement control.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"technique": resourceSchema.StringAttribute{
				MarkdownDescription: "The encoding technique used by the plugin (e.g., 'reed_sol_van' for jerasure). The available techniques depend on the plugin.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"crush_root": resourceSchema.StringAttribute{
				MarkdownDescription: "The CRUSH root for placement. Defaults to 'default' if not specified.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"crush_device_class": resourceSchema.StringAttribute{
				MarkdownDescription: "The device class for placement (e.g., 'ssd', 'hdd'). Restricts the profile to use only OSDs of this device class.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"directory": resourceSchema.StringAttribute{
				MarkdownDescription: "The directory where the erasure code plugin is loaded from (computed by Ceph).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ErasureCodeProfileResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ErasureCodeProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ErasureCodeProfileResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createReq := restapi.ErasureCodeProfileCreateRequest{
		Name: data.Name.ValueString(),
	}

	if !data.K.IsNull() && !data.K.IsUnknown() {
		val := fmt.Sprintf("%d", data.K.ValueInt64())
		createReq.K = &val
	}

	if !data.M.IsNull() && !data.M.IsUnknown() {
		val := fmt.Sprintf("%d", data.M.ValueInt64())
		createReq.M = &val
	}

	if !data.Plugin.IsNull() && !data.Plugin.IsUnknown() {
		val := data.Plugin.ValueString()
		createReq.Plugin = &val
	}

	if !data.CrushFailureDomain.IsNull() && !data.CrushFailureDomain.IsUnknown() {
		val := data.CrushFailureDomain.ValueString()
		createReq.CrushFailureDomain = &val
	}

	if !data.CrushNumFailureDomains.IsNull() && !data.CrushNumFailureDomains.IsUnknown() {
		val := fmt.Sprintf("%d", data.CrushNumFailureDomains.ValueInt64())
		createReq.CrushNumFailureDomains = &val
	}

	if !data.CrushOSDsPerFailureDomain.IsNull() && !data.CrushOSDsPerFailureDomain.IsUnknown() {
		val := fmt.Sprintf("%d", data.CrushOSDsPerFailureDomain.ValueInt64())
		createReq.CrushOSDsPerFailureDomain = &val
	}

	if !data.Technique.IsNull() && !data.Technique.IsUnknown() {
		val := data.Technique.ValueString()
		createReq.Technique = &val
	}

	if !data.CrushRoot.IsNull() && !data.CrushRoot.IsUnknown() {
		val := data.CrushRoot.ValueString()
		createReq.CrushRoot = &val
	}

	if !data.CrushDeviceClass.IsNull() && !data.CrushDeviceClass.IsUnknown() {
		val := data.CrushDeviceClass.ValueString()
		createReq.CrushDeviceClass = &val
	}

	err := r.client.CreateErasureCodeProfile(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create erasure code profile '%s': %s", data.Name.ValueString(), err),
		)
		return
	}

	profile, err := r.client.GetErasureCodeProfile(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read erasure code profile '%s' after creation: %s", data.Name.ValueString(), err),
		)
		return
	}

	r.updateModelFromAPI(&data, profile)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ErasureCodeProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ErasureCodeProfileResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	profile, err := r.client.GetErasureCodeProfile(ctx, data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read erasure code profile '%s': %s", data.Name.ValueString(), err),
		)
		return
	}

	r.updateModelFromAPI(&data, profile)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ErasureCodeProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update Not Supported",
		"Erasure code profiles are immutable in Ceph and cannot be updated. Any changes require replacing the resource.",
	)
}

func (r *ErasureCodeProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ErasureCodeProfileResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteErasureCodeProfile(ctx, data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete erasure code profile '%s': %s. Note that erasure code profiles cannot be deleted if they are in use by any pools.", data.Name.ValueString(), err),
		)
		return
	}
}

func (r *ErasureCodeProfileResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Only guard an in-place replacement (same name, changed parameters): a
	// plain destroy or a rename lets Terraform recreate/remove the referencing
	// pool first. A same-name replace cannot, since Ceph refuses to delete a
	// profile while a pool uses it.
	if r.client == nil || req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var plan, state ErasureCodeProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || plan.Name.ValueString() != state.Name.ValueString() {
		return
	}
	if plan.K.Equal(state.K) &&
		plan.M.Equal(state.M) &&
		plan.Plugin.Equal(state.Plugin) &&
		plan.CrushFailureDomain.Equal(state.CrushFailureDomain) &&
		plan.CrushNumFailureDomains.Equal(state.CrushNumFailureDomains) &&
		plan.CrushOSDsPerFailureDomain.Equal(state.CrushOSDsPerFailureDomain) &&
		plan.Technique.Equal(state.Technique) &&
		plan.CrushRoot.Equal(state.CrushRoot) &&
		plan.CrushDeviceClass.Equal(state.CrushDeviceClass) {
		return
	}

	name := state.Name.ValueString()
	pools, err := r.client.ListPools(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to list pools to check whether erasure code profile '%s' is in use: %s", name, err),
		)
		return
	}

	var inUse []string
	for _, pool := range pools {
		if pool.ErasureCodeProfile == name {
			inUse = append(inUse, pool.PoolName)
		}
	}

	if len(inUse) > 0 {
		resp.Diagnostics.AddError(
			"Erasure Code Profile In Use",
			fmt.Sprintf(
				"Erasure code profile %q is in use by pool(s): %s. Replacing it in place will fail because Ceph cannot delete a profile while a pool uses it, and a pool's profile is fixed at creation. Recreate the pool with a new profile instead.",
				name, strings.Join(inUse, ", "),
			),
		)
	}
}

func (r *ErasureCodeProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *ErasureCodeProfileResource) updateModelFromAPI(data *ErasureCodeProfileResourceModel, profile *restapi.ErasureCodeProfile) {
	data.K = types.Int64Value(int64(profile.K))
	data.M = types.Int64Value(int64(profile.M))
	data.Plugin = types.StringValue(profile.Plugin)
	data.CrushFailureDomain = types.StringValue(profile.CrushFailureDomain)
	if profile.CrushNumFailureDomains != "" {
		if val, err := strconv.ParseInt(profile.CrushNumFailureDomains, 10, 64); err == nil {
			data.CrushNumFailureDomains = types.Int64Value(val)
		} else {
			data.CrushNumFailureDomains = types.Int64Null()
		}
	} else {
		data.CrushNumFailureDomains = types.Int64Null()
	}
	if profile.CrushOSDsPerFailureDomain != "" {
		if val, err := strconv.ParseInt(profile.CrushOSDsPerFailureDomain, 10, 64); err == nil {
			data.CrushOSDsPerFailureDomain = types.Int64Value(val)
		} else {
			data.CrushOSDsPerFailureDomain = types.Int64Null()
		}
	} else {
		data.CrushOSDsPerFailureDomain = types.Int64Null()
	}
	if profile.Technique != "" {
		data.Technique = types.StringValue(profile.Technique)
	} else {
		data.Technique = types.StringNull()
	}
	if profile.CrushRoot != "" {
		data.CrushRoot = types.StringValue(profile.CrushRoot)
	} else {
		data.CrushRoot = types.StringNull()
	}
	if profile.CrushDeviceClass != "" {
		data.CrushDeviceClass = types.StringValue(profile.CrushDeviceClass)
	} else {
		data.CrushDeviceClass = types.StringNull()
	}
	data.Directory = types.StringValue(profile.Directory)
}
