package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &ErasureCodeProfileResource{}
	_ resource.ResourceWithImportState = &ErasureCodeProfileResource{}
)

func newErasureCodeProfileResource() resource.Resource {
	return &ErasureCodeProfileResource{}
}

type ErasureCodeProfileResource struct {
	client *CephAPIClient
}

type ErasureCodeProfileResourceModel struct {
	Name               types.String `tfsdk:"name"`
	K                  types.Int64  `tfsdk:"k"`
	M                  types.Int64  `tfsdk:"m"`
	Plugin             types.String `tfsdk:"plugin"`
	CrushFailureDomain types.String `tfsdk:"crush_failure_domain"`
	Technique          types.String `tfsdk:"technique"`
	CrushRoot          types.String `tfsdk:"crush_root"`
	CrushDeviceClass   types.String `tfsdk:"crush_device_class"`
	Directory          types.String `tfsdk:"directory"`
}

func (r *ErasureCodeProfileResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_erasure_code_profile"
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
				MarkdownDescription: "Number of data chunks. Must be a positive integer. Defaults to 2 if not specified.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"m": resourceSchema.Int64Attribute{
				MarkdownDescription: "Number of coding chunks (parity). Must be a positive integer. Defaults to 1 if not specified.",
				Optional:            true,
				Computed:            true,
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

func (r *ErasureCodeProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ErasureCodeProfileResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createReq := CephAPIErasureCodeProfileCreateRequest{
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
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete erasure code profile '%s': %s. Note that erasure code profiles cannot be deleted if they are in use by any pools.", data.Name.ValueString(), err),
		)
		return
	}
}

func (r *ErasureCodeProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	profileName := req.ID

	profile, err := r.client.GetErasureCodeProfile(ctx, profileName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read erasure code profile '%s' during import: %s", profileName, err),
		)
		return
	}

	var data ErasureCodeProfileResourceModel
	data.Name = types.StringValue(profileName)

	r.updateModelFromAPI(&data, profile)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ErasureCodeProfileResource) updateModelFromAPI(data *ErasureCodeProfileResourceModel, profile *CephAPIErasureCodeProfile) {
	data.K = types.Int64Value(int64(profile.K))
	data.M = types.Int64Value(int64(profile.M))
	data.Plugin = types.StringValue(profile.Plugin)
	data.CrushFailureDomain = types.StringValue(profile.CrushFailureDomain)
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
