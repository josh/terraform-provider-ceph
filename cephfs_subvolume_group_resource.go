package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &CephFSSubvolumeGroupResource{}
	_ resource.ResourceWithImportState = &CephFSSubvolumeGroupResource{}
)

func newCephFSSubvolumeGroupResource() resource.Resource {
	return &CephFSSubvolumeGroupResource{}
}

type CephFSSubvolumeGroupResource struct {
	client *restapi.Client
}

type CephFSSubvolumeGroupResourceModel struct {
	Name       types.String   `tfsdk:"name"`
	VolName    types.String   `tfsdk:"vol_name"`
	Size       types.Int64    `tfsdk:"size"`
	Mode       types.String   `tfsdk:"mode"`
	UID        types.Int64    `tfsdk:"uid"`
	GID        types.Int64    `tfsdk:"gid"`
	PoolLayout types.String   `tfsdk:"pool_layout"`
	Path       types.String   `tfsdk:"path"`
	DataPool   types.String   `tfsdk:"data_pool"`
	Timeouts   timeouts.Value `tfsdk:"timeouts"`
}

func (r *CephFSSubvolumeGroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs_subvolume_group"
}

func (r *CephFSSubvolumeGroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a CephFS subvolume group within a filesystem.",
		Attributes: map[string]resourceSchema.Attribute{
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the subvolume group. Changing the name requires destroying and recreating the subvolume group.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vol_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem. Changing requires destroying and recreating the subvolume group.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The quota size in bytes. When not set, the subvolume group has no quota.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"mode": resourceSchema.StringAttribute{
				MarkdownDescription: "The permissions of the group directory as an octal string without a leading zero, e.g. `755`. Defaults to `755`. Changing requires destroying and recreating the subvolume group.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[1-7][0-7]{2,3}$`),
						"must be an octal permission string without a leading zero, e.g. 755",
					),
				},
			},
			"uid": resourceSchema.Int64Attribute{
				MarkdownDescription: "The owner uid of the group directory. Changing requires destroying and recreating the subvolume group.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"gid": resourceSchema.Int64Attribute{
				MarkdownDescription: "The owner gid of the group directory. Changing requires destroying and recreating the subvolume group.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"pool_layout": resourceSchema.StringAttribute{
				MarkdownDescription: "The data pool the subvolume group stores its data in. Must already be a data pool of the filesystem. When not set, the group inherits the filesystem's default layout; the effective pool is reported by `data_pool`. Changing requires destroying and recreating the subvolume group.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"path": resourceSchema.StringAttribute{
				MarkdownDescription: "The path of the subvolume group within the filesystem.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"data_pool": resourceSchema.StringAttribute{
				MarkdownDescription: "The data pool used by the subvolume group.",
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

func (r *CephFSSubvolumeGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CephFSSubvolumeGroupResource) updateModelFromAPI(data *CephFSSubvolumeGroupResourceModel, info *restapi.SubvolumeInfo) {
	if quota, ok := info.BytesQuotaInt64(); ok && quota > 0 {
		data.Size = types.Int64Value(quota)
	} else {
		data.Size = types.Int64Null()
	}
	// Info returns the full st_mode; only the permission bits are the
	// configured mode.
	data.Mode = types.StringValue(strconv.FormatInt(int64(info.Mode)&0o7777, 8))
	data.UID = types.Int64Value(int64(info.UID))
	data.GID = types.Int64Value(int64(info.GID))
	data.Path = types.StringValue(info.Path)
	data.DataPool = types.StringValue(info.DataPool)
}

func (r *CephFSSubvolumeGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CephFSSubvolumeGroupResourceModel

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

	createReq := restapi.SubvolumeGroupCreateRequest{
		VolName:   data.VolName.ValueString(),
		GroupName: data.Name.ValueString(),
	}

	if !data.Mode.IsNull() && !data.Mode.IsUnknown() {
		createReq.Mode = data.Mode.ValueString()
	}
	if !data.UID.IsNull() && !data.UID.IsUnknown() {
		createReq.UID = data.UID.ValueInt64Pointer()
	}
	if !data.GID.IsNull() && !data.GID.IsUnknown() {
		createReq.GID = data.GID.ValueInt64Pointer()
	}
	if !data.PoolLayout.IsNull() && !data.PoolLayout.IsUnknown() {
		createReq.PoolLayout = data.PoolLayout.ValueString()
	}

	tflog.Debug(ctx, "Creating CephFS subvolume group", map[string]interface{}{
		"vol_name":   data.VolName.ValueString(),
		"group_name": data.Name.ValueString(),
	})

	err := r.client.CephFSSubvolumeGroupCreate(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create CephFS subvolume group '%s' in '%s': %s", data.Name.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	if !data.Size.IsNull() && !data.Size.IsUnknown() {
		updateReq := restapi.SubvolumeGroupUpdateRequest{
			GroupName: data.Name.ValueString(),
			Size:      data.Size.ValueInt64(),
		}

		err = r.client.CephFSSubvolumeGroupUpdate(ctx, data.VolName.ValueString(), updateReq)
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to set size for CephFS subvolume group '%s' in '%s': %s", data.Name.ValueString(), data.VolName.ValueString(), err),
			)
			return
		}
	}

	info, err := r.client.CephFSSubvolumeGroupInfo(ctx, data.VolName.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read CephFS subvolume group '%s' after creation: %s", data.Name.ValueString(), err),
		)
		return
	}

	r.updateModelFromAPI(&data, info)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSSubvolumeGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CephFSSubvolumeGroupResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading CephFS subvolume group", map[string]interface{}{
		"vol_name":   data.VolName.ValueString(),
		"group_name": data.Name.ValueString(),
	})

	info, err := r.client.CephFSSubvolumeGroupInfo(ctx, data.VolName.ValueString(), data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			tflog.Debug(ctx, "CephFS subvolume group not found, removing from state", map[string]interface{}{
				"vol_name":   data.VolName.ValueString(),
				"group_name": data.Name.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read CephFS subvolume group '%s' in '%s': %s", data.Name.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	r.updateModelFromAPI(&data, info)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSSubvolumeGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CephFSSubvolumeGroupResourceModel

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

	tflog.Debug(ctx, "Updating CephFS subvolume group", map[string]interface{}{
		"vol_name":   data.VolName.ValueString(),
		"group_name": data.Name.ValueString(),
	})

	// The dashboard's subvolume group PUT requires a size argument, so only
	// call it when there is a quota to apply; other updates (e.g. timeouts)
	// have nothing to change on the Ceph side.
	if !data.Size.IsNull() && !data.Size.IsUnknown() {
		updateReq := restapi.SubvolumeGroupUpdateRequest{
			GroupName: data.Name.ValueString(),
			Size:      data.Size.ValueInt64(),
		}

		err := r.client.CephFSSubvolumeGroupUpdate(ctx, data.VolName.ValueString(), updateReq)
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to update CephFS subvolume group '%s' in '%s': %s", data.Name.ValueString(), data.VolName.ValueString(), err),
			)
			return
		}
	}

	info, err := r.client.CephFSSubvolumeGroupInfo(ctx, data.VolName.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read CephFS subvolume group '%s' after update: %s", data.Name.ValueString(), err),
		)
		return
	}

	r.updateModelFromAPI(&data, info)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSSubvolumeGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CephFSSubvolumeGroupResourceModel

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

	tflog.Debug(ctx, "Deleting CephFS subvolume group", map[string]interface{}{
		"vol_name":   data.VolName.ValueString(),
		"group_name": data.Name.ValueString(),
	})

	err := r.client.CephFSSubvolumeGroupDelete(ctx, data.VolName.ValueString(), data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete CephFS subvolume group '%s' in '%s': %s", data.Name.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}
}

func (r *CephFSSubvolumeGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: vol_name/group_name, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vol_name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}
