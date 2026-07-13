package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &CephFSSubvolumeSnapshotResource{}
	_ resource.ResourceWithImportState = &CephFSSubvolumeSnapshotResource{}
)

func newCephFSSubvolumeSnapshotResource() resource.Resource {
	return &CephFSSubvolumeSnapshotResource{}
}

type CephFSSubvolumeSnapshotResource struct {
	client *restapi.Client
}

type CephFSSubvolumeSnapshotResourceModel struct {
	VolName    types.String `tfsdk:"vol_name"`
	GroupName  types.String `tfsdk:"group_name"`
	SubvolName types.String `tfsdk:"subvol_name"`
	Name       types.String `tfsdk:"name"`
}

var noSlashValidator = stringvalidator.RegexMatches(
	regexp.MustCompile(`^[^/]+$`),
	"must not contain '/'",
)

func (r *CephFSSubvolumeSnapshotResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs_subvolume_snapshot"
}

func (r *CephFSSubvolumeSnapshotResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a snapshot of a CephFS subvolume. Snapshots are immutable, so any changes trigger resource replacement.",
		Attributes: map[string]resourceSchema.Attribute{
			"vol_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{noSlashValidator},
			},
			"group_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The subvolume group holding the subvolume. When not set, the default group is used.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					noSlashValidator,
				},
			},
			"subvol_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the subvolume to snapshot.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{noSlashValidator},
			},
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the snapshot.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{noSlashValidator},
			},
		},
	}
}

func (r *CephFSSubvolumeSnapshotResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CephFSSubvolumeSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CephFSSubvolumeSnapshotResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.CephFSSubvolumeSnapshotCreate(ctx, restapi.SubvolumeSnapshotCreateRequest{
		VolName:    data.VolName.ValueString(),
		SubvolName: data.SubvolName.ValueString(),
		SnapName:   data.Name.ValueString(),
		GroupName:  data.GroupName.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create snapshot '%s' of subvolume '%s' in '%s': %s", data.Name.ValueString(), data.SubvolName.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSSubvolumeSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CephFSSubvolumeSnapshotResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CephFSSubvolumeSnapshotInfo(ctx, data.VolName.ValueString(), data.SubvolName.ValueString(), data.Name.ValueString(), data.GroupName.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read snapshot '%s' of subvolume '%s' in '%s': %s", data.Name.ValueString(), data.SubvolName.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSSubvolumeSnapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update Not Supported",
		"CephFS subvolume snapshots are immutable and cannot be updated. Any changes require replacing the resource.",
	)
}

func (r *CephFSSubvolumeSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CephFSSubvolumeSnapshotResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.CephFSSubvolumeSnapshotDelete(ctx, data.VolName.ValueString(), data.SubvolName.ValueString(), data.Name.ValueString(), data.GroupName.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete snapshot '%s' of subvolume '%s' in '%s': %s. Note that snapshots with pending clones cannot be deleted.", data.Name.ValueString(), data.SubvolName.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}
}

func (r *CephFSSubvolumeSnapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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
			fmt.Sprintf("Expected format: vol_name/subvol_name/snap_name or vol_name/group_name/subvol_name/snap_name, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vol_name"), parts[0])...)
	if len(parts) == 4 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_name"), parts[1])...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("subvol_name"), parts[len(parts)-2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[len(parts)-1])...)
}
