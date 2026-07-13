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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &CephFSSubvolumeResource{}
	_ resource.ResourceWithImportState = &CephFSSubvolumeResource{}
)

func newCephFSSubvolumeResource() resource.Resource {
	return &CephFSSubvolumeResource{}
}

type CephFSSubvolumeResource struct {
	client *restapi.Client
}

type CephFSSubvolumeResourceModel struct {
	Name              types.String   `tfsdk:"name"`
	VolName           types.String   `tfsdk:"vol_name"`
	GroupName         types.String   `tfsdk:"group_name"`
	Size              types.Int64    `tfsdk:"size"`
	Mode              types.String   `tfsdk:"mode"`
	UID               types.Int64    `tfsdk:"uid"`
	GID               types.Int64    `tfsdk:"gid"`
	PoolLayout        types.String   `tfsdk:"pool_layout"`
	NamespaceIsolated types.Bool     `tfsdk:"namespace_isolated"`
	Earmark           types.String   `tfsdk:"earmark"`
	Path              types.String   `tfsdk:"path"`
	DataPool          types.String   `tfsdk:"data_pool"`
	PoolNamespace     types.String   `tfsdk:"pool_namespace"`
	State             types.String   `tfsdk:"state"`
	Timeouts          timeouts.Value `tfsdk:"timeouts"`
}

func (r *CephFSSubvolumeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs_subvolume"
}

func (r *CephFSSubvolumeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a CephFS subvolume within a filesystem.",
		Attributes: map[string]resourceSchema.Attribute{
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the subvolume. Changing the name requires destroying and recreating the subvolume.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vol_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem. Changing requires destroying and recreating the subvolume.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The subvolume group holding the subvolume. When not set, the subvolume lives in the default group. Changing requires destroying and recreating the subvolume.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					noSlashValidator,
				},
			},
			"size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The quota size in bytes. When not set, the subvolume has no quota.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"mode": resourceSchema.StringAttribute{
				MarkdownDescription: "The permissions of the subvolume directory as an octal string without a leading zero, e.g. `755`. Defaults to `755`. Changing requires destroying and recreating the subvolume.",
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
				MarkdownDescription: "The owner uid of the subvolume directory. Changing requires destroying and recreating the subvolume.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"gid": resourceSchema.Int64Attribute{
				MarkdownDescription: "The owner gid of the subvolume directory. Changing requires destroying and recreating the subvolume.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"pool_layout": resourceSchema.StringAttribute{
				MarkdownDescription: "The data pool the subvolume stores its data in. Must already be a data pool of the filesystem. When not set, the subvolume inherits its parent's layout; the effective pool is reported by `data_pool`. Changing requires destroying and recreating the subvolume.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"namespace_isolated": resourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the subvolume stores its data in a dedicated RADOS namespace, reported by `pool_namespace`. Changing requires destroying and recreating the subvolume.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			// The dashboard only accepts earmark at creation time; the
			// `ceph fs subvolume earmark set` CLI can still change it out
			// of band.
			"earmark": resourceSchema.StringAttribute{
				MarkdownDescription: "The earmark tagging the subvolume for a consumer, scoped under `nfs` or `smb` (e.g. `smb` or `smb.cluster.mycluster`). Changing requires destroying and recreating the subvolume.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^(nfs|smb)(\..+)?$`),
						"must be nfs or smb, optionally followed by dot-separated subparts",
					),
				},
			},
			"path": resourceSchema.StringAttribute{
				MarkdownDescription: "The path of the subvolume within the filesystem.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"pool_namespace": resourceSchema.StringAttribute{
				MarkdownDescription: "The RADOS namespace holding the subvolume data when the subvolume is namespace isolated.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"data_pool": resourceSchema.StringAttribute{
				MarkdownDescription: "The data pool used by the subvolume.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"state": resourceSchema.StringAttribute{
				MarkdownDescription: "The state of the subvolume.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *CephFSSubvolumeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CephFSSubvolumeResource) updateModelFromAPI(data *CephFSSubvolumeResourceModel, info *restapi.SubvolumeInfo) {
	data.Path = types.StringValue(info.Path)
	data.DataPool = types.StringValue(info.DataPool)
	data.State = types.StringValue(info.State)
	data.NamespaceIsolated = types.BoolValue(info.PoolNamespace != "")
	if info.PoolNamespace != "" {
		data.PoolNamespace = types.StringValue(info.PoolNamespace)
	} else {
		data.PoolNamespace = types.StringNull()
	}
	if !data.Earmark.IsNull() || info.Earmark != "" {
		data.Earmark = types.StringValue(info.Earmark)
	}
	// Info returns the full st_mode; only the permission bits are the
	// configured mode.
	data.Mode = types.StringValue(strconv.FormatInt(int64(info.Mode)&0o7777, 8))
	data.UID = types.Int64Value(int64(info.UID))
	data.GID = types.Int64Value(int64(info.GID))

	if quota, ok := info.BytesQuotaInt64(); ok && quota > 0 {
		data.Size = types.Int64Value(quota)
	} else {
		data.Size = types.Int64Null()
	}
}

func (r *CephFSSubvolumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CephFSSubvolumeResourceModel

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

	createReq := restapi.SubvolumeCreateRequest{
		VolName:    data.VolName.ValueString(),
		SubvolName: data.Name.ValueString(),
		GroupName:  data.GroupName.ValueString(),
	}

	if !data.Size.IsNull() && !data.Size.IsUnknown() {
		createReq.Size = data.Size.ValueInt64()
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
	if !data.NamespaceIsolated.IsNull() && !data.NamespaceIsolated.IsUnknown() && data.NamespaceIsolated.ValueBool() {
		createReq.NamespaceIsolated = data.NamespaceIsolated.ValueBoolPointer()
	}
	if !data.Earmark.IsNull() && !data.Earmark.IsUnknown() {
		createReq.Earmark = data.Earmark.ValueString()
	}

	tflog.Debug(ctx, "Creating CephFS subvolume", map[string]interface{}{
		"vol_name":    data.VolName.ValueString(),
		"subvol_name": data.Name.ValueString(),
	})

	err := r.client.CephFSSubvolumeCreate(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create CephFS subvolume '%s' in '%s': %s", data.Name.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	info, err := r.client.CephFSSubvolumeInfo(ctx, data.VolName.ValueString(), data.Name.ValueString(), data.GroupName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read CephFS subvolume '%s' after creation: %s", data.Name.ValueString(), err),
		)
		return
	}

	r.updateModelFromAPI(&data, info)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSSubvolumeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CephFSSubvolumeResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading CephFS subvolume", map[string]interface{}{
		"vol_name":    data.VolName.ValueString(),
		"subvol_name": data.Name.ValueString(),
	})

	info, err := r.client.CephFSSubvolumeInfo(ctx, data.VolName.ValueString(), data.Name.ValueString(), data.GroupName.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			tflog.Debug(ctx, "CephFS subvolume not found, removing from state", map[string]interface{}{
				"vol_name":    data.VolName.ValueString(),
				"subvol_name": data.Name.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read CephFS subvolume '%s' in '%s': %s", data.Name.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	r.updateModelFromAPI(&data, info)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSSubvolumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CephFSSubvolumeResourceModel

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

	if !data.Size.IsNull() && !data.Size.IsUnknown() {
		updateReq := restapi.SubvolumeUpdateRequest{
			SubvolName: data.Name.ValueString(),
			Size:       data.Size.ValueInt64(),
			GroupName:  data.GroupName.ValueString(),
		}

		tflog.Debug(ctx, "Updating CephFS subvolume", map[string]interface{}{
			"vol_name":    data.VolName.ValueString(),
			"subvol_name": data.Name.ValueString(),
			"size":        data.Size.ValueInt64(),
		})

		err := r.client.CephFSSubvolumeUpdate(ctx, data.VolName.ValueString(), updateReq)
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to update CephFS subvolume '%s' in '%s': %s", data.Name.ValueString(), data.VolName.ValueString(), err),
			)
			return
		}
	}

	info, err := r.client.CephFSSubvolumeInfo(ctx, data.VolName.ValueString(), data.Name.ValueString(), data.GroupName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read CephFS subvolume '%s' in '%s': %s", data.Name.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	r.updateModelFromAPI(&data, info)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSSubvolumeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CephFSSubvolumeResourceModel

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

	tflog.Debug(ctx, "Deleting CephFS subvolume", map[string]interface{}{
		"vol_name":    data.VolName.ValueString(),
		"subvol_name": data.Name.ValueString(),
	})

	err := r.client.CephFSSubvolumeDelete(ctx, data.VolName.ValueString(), data.Name.ValueString(), data.GroupName.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete CephFS subvolume '%s' in '%s': %s", data.Name.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}
}

func (r *CephFSSubvolumeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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
			fmt.Sprintf("Expected format: vol_name/subvol_name or vol_name/group_name/subvol_name, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vol_name"), parts[0])...)
	if len(parts) == 3 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_name"), parts[1])...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[len(parts)-1])...)
}
