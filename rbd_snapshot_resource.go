package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &RBDSnapshotResource{}
	_ resource.ResourceWithImportState = &RBDSnapshotResource{}
)

func newRBDSnapshotResource() resource.Resource {
	return &RBDSnapshotResource{}
}

type RBDSnapshotResource struct {
	client *restapi.Client
}

type RBDSnapshotResourceModel struct {
	PoolName    types.String   `tfsdk:"pool_name"`
	Namespace   types.String   `tfsdk:"namespace"`
	ImageName   types.String   `tfsdk:"image_name"`
	Name        types.String   `tfsdk:"name"`
	IsProtected types.Bool     `tfsdk:"is_protected"`
	Size        types.Int64    `tfsdk:"size"`
	Timestamp   types.String   `tfsdk:"timestamp"`
	Timeouts    timeouts.Value `tfsdk:"timeouts"`
}

func rbdImageSpec(poolName, namespace, imageName string) string {
	if namespace != "" {
		return poolName + "/" + namespace + "/" + imageName
	}
	return poolName + "/" + imageName
}

func (r *RBDSnapshotResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rbd_snapshot"
}

func (r *RBDSnapshotResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a snapshot of an RBD image.",
		Attributes: map[string]resourceSchema.Attribute{
			"pool_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the pool holding the image. Changing requires destroying and recreating the snapshot.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"namespace": resourceSchema.StringAttribute{
				MarkdownDescription: "The RBD namespace holding the image. Changing requires destroying and recreating the snapshot.",
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
			"image_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the image to snapshot. Changing requires destroying and recreating the snapshot.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the snapshot. Changing renames the snapshot in place.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[^/@]+$`),
						"must not contain '/' or '@'",
					),
				},
			},
			"is_protected": resourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the snapshot is protected from deletion. Protection is required before cloning. Defaults to false.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The size of the image at snapshot time in bytes.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"timestamp": resourceSchema.StringAttribute{
				MarkdownDescription: "The snapshot creation timestamp.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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

func (r *RBDSnapshotResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RBDSnapshotResource) updateModelFromAPI(data *RBDSnapshotResourceModel, snap *restapi.RBDImageSnapshot) {
	data.Size = types.Int64Value(snap.Size)
	data.Timestamp = types.StringValue(snap.Timestamp)
	data.IsProtected = types.BoolValue(snap.IsProtected != nil && *snap.IsProtected)
}

func (r *RBDSnapshotResource) waitForTask(ctx context.Context, taskInfo *restapi.TaskInfo, action string, diags *diag.Diagnostics) bool {
	if taskInfo == nil {
		return true
	}

	tflog.Debug(ctx, "RBD snapshot "+action+" is async, waiting for task", map[string]interface{}{
		"task_name": taskInfo.Name,
	})

	if err := r.client.WaitForTask(ctx, taskInfo); err != nil {
		diags.AddError(
			"Task Wait Failed",
			fmt.Sprintf("Failed waiting for RBD snapshot %s task: %s", action, err),
		)
		return false
	}
	return true
}

func (r *RBDSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RBDSnapshotResourceModel

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
	namespace := data.Namespace.ValueString()
	imageName := data.ImageName.ValueString()
	snapName := data.Name.ValueString()
	imageSpec := rbdImageSpec(poolName, namespace, imageName)

	taskInfo, err := r.client.CreateRBDSnapshot(ctx, poolName, namespace, imageName, snapName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create snapshot '%s' of RBD image '%s': %s", snapName, imageSpec, err),
		)
		return
	}

	// The snapshot may exist from here on, so record it before the wait and
	// read back to keep a failure there from orphaning it from state.
	partial := data
	partial.Size = types.Int64Null()
	partial.Timestamp = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &partial)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.waitForTask(ctx, taskInfo, "creation", &resp.Diagnostics) {
		return
	}

	if data.IsProtected.ValueBool() {
		protect := true
		taskInfo, err := r.client.UpdateRBDSnapshot(ctx, poolName, namespace, imageName, snapName, restapi.RBDSnapshotUpdateRequest{
			IsProtected: &protect,
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to protect snapshot '%s' of RBD image '%s': %s", snapName, imageSpec, err),
			)
			return
		}
		if !r.waitForTask(ctx, taskInfo, "protection", &resp.Diagnostics) {
			return
		}
	}

	snap, err := r.client.GetRBDSnapshot(ctx, poolName, namespace, imageName, snapName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read snapshot '%s' of RBD image '%s' after creation: %s", snapName, imageSpec, err),
		)
		return
	}

	r.updateModelFromAPI(&data, snap)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RBDSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RBDSnapshotResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	snap, err := r.client.GetRBDSnapshot(ctx, data.PoolName.ValueString(), data.Namespace.ValueString(), data.ImageName.ValueString(), data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read snapshot '%s' of RBD image '%s': %s", data.Name.ValueString(), rbdImageSpec(data.PoolName.ValueString(), data.Namespace.ValueString(), data.ImageName.ValueString()), err),
		)
		return
	}

	r.updateModelFromAPI(&data, snap)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RBDSnapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RBDSnapshotResourceModel
	var state RBDSnapshotResourceModel

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

	poolName := state.PoolName.ValueString()
	namespace := state.Namespace.ValueString()
	imageName := state.ImageName.ValueString()
	imageSpec := rbdImageSpec(poolName, namespace, imageName)

	updateReq := restapi.RBDSnapshotUpdateRequest{}
	if !data.Name.Equal(state.Name) {
		updateReq.NewSnapName = data.Name.ValueStringPointer()
	}
	if !data.IsProtected.Equal(state.IsProtected) {
		updateReq.IsProtected = data.IsProtected.ValueBoolPointer()
	}

	taskInfo, err := r.client.UpdateRBDSnapshot(ctx, poolName, namespace, imageName, state.Name.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update snapshot '%s' of RBD image '%s': %s", state.Name.ValueString(), imageSpec, err),
		)
		return
	}

	if !r.waitForTask(ctx, taskInfo, "update", &resp.Diagnostics) {
		return
	}

	snap, err := r.client.GetRBDSnapshot(ctx, poolName, namespace, imageName, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read snapshot '%s' of RBD image '%s' after update: %s", data.Name.ValueString(), imageSpec, err),
		)
		return
	}

	r.updateModelFromAPI(&data, snap)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RBDSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RBDSnapshotResourceModel

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
	namespace := data.Namespace.ValueString()
	imageName := data.ImageName.ValueString()
	snapName := data.Name.ValueString()
	imageSpec := rbdImageSpec(poolName, namespace, imageName)

	if data.IsProtected.ValueBool() {
		unprotect := false
		taskInfo, err := r.client.UpdateRBDSnapshot(ctx, poolName, namespace, imageName, snapName, restapi.RBDSnapshotUpdateRequest{
			IsProtected: &unprotect,
		})
		if err != nil && !errors.Is(err, restapi.ErrNotFound) {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to unprotect snapshot '%s' of RBD image '%s' before deletion: %s. Note that snapshots with dependent clones cannot be unprotected.", snapName, imageSpec, err),
			)
			return
		}
		if err == nil && !r.waitForTask(ctx, taskInfo, "unprotection", &resp.Diagnostics) {
			return
		}
	}

	taskInfo, err := r.client.DeleteRBDSnapshot(ctx, poolName, namespace, imageName, snapName)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete snapshot '%s' of RBD image '%s': %s", snapName, imageSpec, err),
		)
		return
	}

	r.waitForTask(ctx, taskInfo, "deletion", &resp.Diagnostics)
}

func (r *RBDSnapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	valid := len(parts) == 3 || len(parts) == 4
	for _, part := range parts {
		if part == "" {
			valid = false
		}
	}
	if !valid {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: pool_name/image_name/snapshot_name or pool_name/namespace/image_name/snapshot_name, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_name"), parts[0])...)
	if len(parts) == 4 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("namespace"), parts[1])...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("image_name"), parts[len(parts)-2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[len(parts)-1])...)
}
