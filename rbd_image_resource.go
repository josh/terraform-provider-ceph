package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &RBDImageResource{}
	_ resource.ResourceWithImportState = &RBDImageResource{}
)

func newRBDImageResource() resource.Resource {
	return &RBDImageResource{}
}

type RBDImageResource struct {
	client *restapi.Client
}

type RBDImageResourceModel struct {
	PoolName        types.String   `tfsdk:"pool_name"`
	Namespace       types.String   `tfsdk:"namespace"`
	Name            types.String   `tfsdk:"name"`
	Size            types.Int64    `tfsdk:"size"`
	ObjectSize      types.Int64    `tfsdk:"object_size"`
	DataPool        types.String   `tfsdk:"data_pool"`
	Features        types.Set      `tfsdk:"features"`
	ID              types.String   `tfsdk:"id"`
	BlockNamePrefix types.String   `tfsdk:"block_name_prefix"`
	FeaturesName    types.Set      `tfsdk:"features_name"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

// Feature toggles the dashboard applies in place; deltas outside these
// sets are silently ignored by Ceph, so they force a replacement
// (src/pybind/mgr/dashboard/services/rbd.py ALLOW_ENABLE_FEATURES and
// ALLOW_DISABLE_FEATURES).
var (
	rbdImageEnableableFeatures  = map[string]bool{"exclusive-lock": true, "object-map": true, "fast-diff": true, "journaling": true}
	rbdImageDisableableFeatures = map[string]bool{"exclusive-lock": true, "object-map": true, "fast-diff": true, "deep-flatten": true, "journaling": true}
)

func rbdImageFeaturesRequireReplace(ctx context.Context, req planmodifier.SetRequest, resp *setplanmodifier.RequiresReplaceIfFuncResponse) {
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}

	var current types.Set
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("features_name"), &current)...)
	if resp.Diagnostics.HasError() || current.IsNull() || current.IsUnknown() {
		return
	}

	var currentNames, planned []string
	resp.Diagnostics.Append(current.ElementsAs(ctx, &currentNames, false)...)
	resp.Diagnostics.Append(req.PlanValue.ElementsAs(ctx, &planned, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, feature := range planned {
		if !slices.Contains(currentNames, feature) && !rbdImageEnableableFeatures[feature] {
			resp.RequiresReplace = true
			return
		}
	}
	for _, feature := range currentNames {
		if !slices.Contains(planned, feature) && !rbdImageDisableableFeatures[feature] {
			resp.RequiresReplace = true
			return
		}
	}
}

func (r *RBDImageResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rbd_image"
}

func (r *RBDImageResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages an RBD image within a pool.",
		Attributes: map[string]resourceSchema.Attribute{
			"pool_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the pool holding the image. Changing requires destroying and recreating the image.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"namespace": resourceSchema.StringAttribute{
				MarkdownDescription: "The RBD namespace holding the image. Changing requires destroying and recreating the image.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[^/@]+$`),
						"must not contain '/' or '@'",
					),
				},
			},
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the image. Changing renames the image in place.",
				Required:            true,
			},
			"size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The size of the image in bytes. Shrinking an image discards data beyond the new size.",
				Required:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"object_size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The object size of the image in bytes. Must be a power of two, since Ceph rounds any other value to the nearest one. When not set, Ceph uses its default. Changing requires destroying and recreating the image.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplaceIfConfigured(),
				},
				Validators: []validator.Int64{
					powerOfTwo(),
				},
			},
			"data_pool": resourceSchema.StringAttribute{
				MarkdownDescription: "The erasure coded pool holding the image data. Changing requires destroying and recreating the image.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"features": resourceSchema.SetAttribute{
				MarkdownDescription: "The features to enable on the image. When not set, Ceph uses its default features; an empty set creates the image with no features. Only `exclusive-lock`, `object-map`, `fast-diff` and `journaling` can be enabled and `deep-flatten` additionally disabled on an existing image; other changes require destroying and recreating the image.",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.OneOf("layering", "striping", "exclusive-lock", "object-map", "fast-diff", "deep-flatten", "journaling", "data-pool"),
					),
				},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplaceIf(
						rbdImageFeaturesRequireReplace,
						"Feature changes Ceph cannot apply in place require replacement.",
						"Feature changes Ceph cannot apply in place require replacement.",
					),
				},
			},
			"id": resourceSchema.StringAttribute{
				MarkdownDescription: "The internal id of the image.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"block_name_prefix": resourceSchema.StringAttribute{
				MarkdownDescription: "The prefix of the RADOS objects backing the image.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"features_name": resourceSchema.SetAttribute{
				MarkdownDescription: "The features enabled on the image.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *RBDImageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RBDImageResource) updateModelFromAPI(ctx context.Context, data *RBDImageResourceModel, image *restapi.RBDImage) diag.Diagnostics {
	var diags diag.Diagnostics

	if !data.Namespace.IsNull() || image.Namespace != "" {
		data.Namespace = types.StringValue(image.Namespace)
	}
	data.Size = types.Int64Value(image.Size)
	data.ObjectSize = types.Int64Value(image.ObjSize)
	data.DataPool = types.StringPointerValue(image.DataPool)
	data.ID = types.StringValue(image.ID)
	data.BlockNamePrefix = types.StringValue(image.BlockNamePrefix)

	featuresName, d := types.SetValueFrom(ctx, types.StringType, image.FeaturesName)
	diags.Append(d...)
	data.FeaturesName = featuresName

	return diags
}

func (r *RBDImageResource) waitForTask(ctx context.Context, taskInfo *restapi.TaskInfo, action string, diags *diag.Diagnostics) bool {
	if taskInfo == nil {
		return true
	}

	tflog.Debug(ctx, "RBD image "+action+" is async, waiting for task", map[string]interface{}{
		"task_name": taskInfo.Name,
	})

	if err := r.client.WaitForTask(ctx, taskInfo); err != nil {
		diags.AddError(
			"Task Wait Failed",
			fmt.Sprintf("Failed waiting for RBD image %s task: %s", action, err),
		)
		return false
	}
	return true
}

func (r *RBDImageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RBDImageResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := data.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	createReq := restapi.RBDImageCreateRequest{
		Name:     data.Name.ValueString(),
		PoolName: data.PoolName.ValueString(),
		Size:     data.Size.ValueInt64(),
	}

	if !data.Namespace.IsNull() && !data.Namespace.IsUnknown() {
		createReq.Namespace = data.Namespace.ValueStringPointer()
	}

	if !data.ObjectSize.IsNull() && !data.ObjectSize.IsUnknown() {
		createReq.ObjSize = data.ObjectSize.ValueInt64Pointer()
	}
	if !data.DataPool.IsNull() && !data.DataPool.IsUnknown() {
		createReq.DataPool = data.DataPool.ValueStringPointer()
	}
	if !data.Features.IsNull() && !data.Features.IsUnknown() {
		resp.Diagnostics.Append(data.Features.ElementsAs(ctx, &createReq.Features, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	tflog.Debug(ctx, "Creating RBD image", map[string]interface{}{
		"pool_name": data.PoolName.ValueString(),
		"name":      data.Name.ValueString(),
		"size":      data.Size.ValueInt64(),
	})

	taskInfo, err := r.client.CreateRBDImage(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create RBD image '%s' in pool '%s': %s", data.Name.ValueString(), data.PoolName.ValueString(), err),
		)
		return
	}

	// The image may exist from here on, so record it before the wait and
	// read back to keep a failure there from orphaning it from state.
	partial := data
	if partial.ObjectSize.IsUnknown() {
		partial.ObjectSize = types.Int64Null()
	}
	if partial.DataPool.IsUnknown() {
		partial.DataPool = types.StringNull()
	}
	partial.ID = types.StringNull()
	partial.BlockNamePrefix = types.StringNull()
	partial.FeaturesName = types.SetNull(types.StringType)
	resp.Diagnostics.Append(resp.State.Set(ctx, &partial)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.waitForTask(ctx, taskInfo, "creation", &resp.Diagnostics) {
		return
	}

	image, err := r.client.GetRBDImage(ctx, data.PoolName.ValueString(), data.Namespace.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RBD image '%s' after creation: %s", data.Name.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, image)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RBDImageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RBDImageResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading RBD image", map[string]interface{}{
		"pool_name": data.PoolName.ValueString(),
		"name":      data.Name.ValueString(),
	})

	image, err := r.client.GetRBDImage(ctx, data.PoolName.ValueString(), data.Namespace.ValueString(), data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			tflog.Debug(ctx, "RBD image not found, removing from state", map[string]interface{}{
				"pool_name": data.PoolName.ValueString(),
				"name":      data.Name.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RBD image '%s' in pool '%s': %s", data.Name.ValueString(), data.PoolName.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, image)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RBDImageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RBDImageResourceModel
	var state RBDImageResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, diags := data.Timeouts.Update(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	updateReq := restapi.RBDImageUpdateRequest{}

	if !data.Name.Equal(state.Name) {
		updateReq.Name = data.Name.ValueStringPointer()
	}
	if !data.Size.Equal(state.Size) {
		updateReq.Size = data.Size.ValueInt64Pointer()
	}
	if !data.Features.IsNull() && !data.Features.IsUnknown() && !data.Features.Equal(state.Features) {
		resp.Diagnostics.Append(data.Features.ElementsAs(ctx, &updateReq.Features, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	tflog.Debug(ctx, "Updating RBD image", map[string]interface{}{
		"pool_name": state.PoolName.ValueString(),
		"name":      state.Name.ValueString(),
	})

	taskInfo, err := r.client.UpdateRBDImage(ctx, state.PoolName.ValueString(), state.Namespace.ValueString(), state.Name.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update RBD image '%s' in pool '%s': %s", state.Name.ValueString(), state.PoolName.ValueString(), err),
		)
		return
	}

	if !r.waitForTask(ctx, taskInfo, "update", &resp.Diagnostics) {
		return
	}

	image, err := r.client.GetRBDImage(ctx, data.PoolName.ValueString(), data.Namespace.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RBD image '%s' after update: %s", data.Name.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, image)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RBDImageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RBDImageResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, diags := data.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	tflog.Debug(ctx, "Deleting RBD image", map[string]interface{}{
		"pool_name": data.PoolName.ValueString(),
		"name":      data.Name.ValueString(),
	})

	taskInfo, err := r.client.DeleteRBDImage(ctx, data.PoolName.ValueString(), data.Namespace.ValueString(), data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete RBD image '%s' in pool '%s': %s", data.Name.ValueString(), data.PoolName.ValueString(), err),
		)
		return
	}

	r.waitForTask(ctx, taskInfo, "deletion", &resp.Diagnostics)
}

func (r *RBDImageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	valid := len(parts) == 2 || len(parts) == 3
	for _, part := range parts {
		if part == "" {
			valid = false
		}
	}
	if !valid {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: pool_name/image_name or pool_name/namespace/image_name, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_name"), parts[0])...)
	if len(parts) == 3 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("namespace"), parts[1])...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[len(parts)-1])...)
}
