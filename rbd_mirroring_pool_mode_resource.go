package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &RBDMirroringPoolModeResource{}
	_ resource.ResourceWithImportState = &RBDMirroringPoolModeResource{}
)

func newRBDMirroringPoolModeResource() resource.Resource {
	return &RBDMirroringPoolModeResource{}
}

type RBDMirroringPoolModeResource struct {
	client *restapi.Client
}

type RBDMirroringPoolModeResourceModel struct {
	PoolName types.String   `tfsdk:"pool_name"`
	Mode     types.String   `tfsdk:"mode"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func (r *RBDMirroringPoolModeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rbd_mirroring_pool_mode"
}

func (r *RBDMirroringPoolModeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages the RBD mirroring mode of a pool. A pool always has a mirroring mode; creating this resource sets it and destroying it resets the mode to `disabled`. Note that disabling can fail while images in the pool still have mirroring enabled.",
		Attributes: map[string]resourceSchema.Attribute{
			"pool_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the pool.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mode": resourceSchema.StringAttribute{
				MarkdownDescription: "The mirroring mode. `pool` mirrors all images with the journaling feature, `image` mirrors only explicitly enabled images. To disable mirroring, destroy the resource.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("pool", "image"),
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *RBDMirroringPoolModeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RBDMirroringPoolModeResource) waitForTask(ctx context.Context, taskInfo *restapi.TaskInfo, action string, diags *diag.Diagnostics) bool {
	if taskInfo == nil {
		return true
	}

	tflog.Debug(ctx, "RBD mirroring pool mode "+action+" is async, waiting for task", map[string]interface{}{
		"task_name": taskInfo.Name,
	})

	if err := r.client.WaitForTask(ctx, taskInfo); err != nil {
		diags.AddError(
			"Task Wait Failed",
			fmt.Sprintf("Failed waiting for RBD mirroring pool mode %s task: %s", action, err),
		)
		return false
	}
	return true
}

func (r *RBDMirroringPoolModeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RBDMirroringPoolModeResourceModel

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

	poolName := data.PoolName.ValueString()

	taskInfo, err := r.client.SetRBDMirroringPoolMode(ctx, poolName, data.Mode.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to set mirroring mode on pool '%s': %s", poolName, err),
		)
		return
	}

	// The mode may be applied from here on, so record state before the
	// wait to keep a failure there from orphaning it.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.waitForTask(ctx, taskInfo, "creation", &resp.Diagnostics) {
		return
	}

	mode, err := r.client.GetRBDMirroringPoolMode(ctx, poolName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read mirroring mode on pool '%s' after setting it: %s", poolName, err),
		)
		return
	}

	data.Mode = types.StringValue(mode.MirrorMode)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RBDMirroringPoolModeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RBDMirroringPoolModeResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mode, err := r.client.GetRBDMirroringPoolMode(ctx, data.PoolName.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read mirroring mode on pool '%s': %s", data.PoolName.ValueString(), err),
		)
		return
	}

	// The returned mode may be "disabled" after an out-of-band disable;
	// state values bypass config validation, so storing it surfaces the
	// drift as an update plan back to the configured mode.
	data.Mode = types.StringValue(mode.MirrorMode)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RBDMirroringPoolModeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RBDMirroringPoolModeResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
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

	poolName := data.PoolName.ValueString()

	taskInfo, err := r.client.SetRBDMirroringPoolMode(ctx, poolName, data.Mode.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to set mirroring mode on pool '%s': %s", poolName, err),
		)
		return
	}

	if !r.waitForTask(ctx, taskInfo, "update", &resp.Diagnostics) {
		return
	}

	mode, err := r.client.GetRBDMirroringPoolMode(ctx, poolName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read mirroring mode on pool '%s' after setting it: %s", poolName, err),
		)
		return
	}

	data.Mode = types.StringValue(mode.MirrorMode)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RBDMirroringPoolModeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RBDMirroringPoolModeResourceModel

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

	poolName := data.PoolName.ValueString()

	taskInfo, err := r.client.SetRBDMirroringPoolMode(ctx, poolName, "disabled")
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to disable mirroring on pool '%s': %s. Note that mirroring cannot be disabled while images in the pool have mirroring enabled.", poolName, err),
		)
		return
	}

	r.waitForTask(ctx, taskInfo, "deletion", &resp.Diagnostics)
}

func (r *RBDMirroringPoolModeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_name"), req.ID)...)
}
