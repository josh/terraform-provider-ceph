package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &CephFSSubvolumeGroupResource{}
	_ resource.ResourceWithImportState = &CephFSSubvolumeGroupResource{}
)

func newCephFSSubvolumeGroupResource() resource.Resource {
	return &CephFSSubvolumeGroupResource{}
}

type CephFSSubvolumeGroupResource struct {
	client *CephAPIClient
}

type CephFSSubvolumeGroupResourceModel struct {
	Name     types.String   `tfsdk:"name"`
	VolName  types.String   `tfsdk:"vol_name"`
	Size     types.Int64    `tfsdk:"size"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
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

func (r *CephFSSubvolumeGroupResource) updateModelFromAPI(data *CephFSSubvolumeGroupResourceModel, info *CephAPISubvolumeInfo) {
	if quota, ok := info.BytesQuotaInt64(); ok && quota > 0 {
		data.Size = types.Int64Value(quota)
	} else {
		data.Size = types.Int64Null()
	}
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

	createReq := CephAPISubvolumeGroupCreateRequest{
		VolName:   data.VolName.ValueString(),
		GroupName: data.Name.ValueString(),
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
		updateReq := CephAPISubvolumeGroupUpdateRequest{
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
		if errors.Is(err, ErrAPINotFound) {
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
		updateReq := CephAPISubvolumeGroupUpdateRequest{
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
		if errors.Is(err, ErrAPINotFound) {
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
