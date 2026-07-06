package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
	Name     types.String   `tfsdk:"name"`
	VolName  types.String   `tfsdk:"vol_name"`
	Size     types.Int64    `tfsdk:"size"`
	Path     types.String   `tfsdk:"path"`
	DataPool types.String   `tfsdk:"data_pool"`
	State    types.String   `tfsdk:"state"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
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
			"size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The quota size in bytes. When not set, the subvolume has no quota.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"path": resourceSchema.StringAttribute{
				MarkdownDescription: "The path of the subvolume within the filesystem.",
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
	}

	if !data.Size.IsNull() && !data.Size.IsUnknown() {
		createReq.Size = data.Size.ValueInt64()
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

	info, err := r.client.CephFSSubvolumeInfo(ctx, data.VolName.ValueString(), data.Name.ValueString(), "")
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

	info, err := r.client.CephFSSubvolumeInfo(ctx, data.VolName.ValueString(), data.Name.ValueString(), "")
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

	info, err := r.client.CephFSSubvolumeInfo(ctx, data.VolName.ValueString(), data.Name.ValueString(), "")
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

	err := r.client.CephFSSubvolumeDelete(ctx, data.VolName.ValueString(), data.Name.ValueString(), "")
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
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: vol_name/subvol_name, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vol_name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}
