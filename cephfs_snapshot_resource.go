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
	_ resource.Resource                = &CephFSSnapshotResource{}
	_ resource.ResourceWithImportState = &CephFSSnapshotResource{}
)

func newCephFSSnapshotResource() resource.Resource {
	return &CephFSSnapshotResource{}
}

type CephFSSnapshotResource struct {
	client *restapi.Client
}

type CephFSSnapshotResourceModel struct {
	VolName types.String `tfsdk:"vol_name"`
	Path    types.String `tfsdk:"path"`
	Name    types.String `tfsdk:"name"`
	Created types.String `tfsdk:"created"`
}

var noColonOrSlashValidator = stringvalidator.RegexMatches(
	regexp.MustCompile(`^[^/:]+$`),
	"must not contain '/' or ':'",
)

func (r *CephFSSnapshotResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs_snapshot"
}

func (r *CephFSSnapshotResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a snapshot of a CephFS directory. Snapshots are immutable, so any changes trigger resource replacement. For snapshots of subvolumes managed by the volumes module, use `ceph_cephfs_subvolume_snapshot` instead.",
		Attributes: map[string]resourceSchema.Attribute{
			"vol_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{noColonOrSlashValidator},
			},
			"path": resourceSchema.StringAttribute{
				MarkdownDescription: "The directory path to snapshot.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					cephFSDirectoryPath(),
				},
			},
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the snapshot.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					noColonOrSlashValidator,
					// Reads filter names starting with '_' (reserved for
					// subvolume-internal snapshots), so such a snapshot
					// would never converge.
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[^_]`),
						"must not start with '_'",
					),
				},
			},
			"created": resourceSchema.StringAttribute{
				MarkdownDescription: "The snapshot creation timestamp.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *CephFSSnapshotResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CephFSSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CephFSSnapshotResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	volName := data.VolName.ValueString()
	dirPath := data.Path.ValueString()
	snapName := data.Name.ValueString()

	fs, err := r.client.CephFSGetByName(ctx, volName)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("CephFS filesystem '%s' does not exist", volName),
			)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to look up CephFS filesystem '%s': %s", volName, err),
		)
		return
	}

	err = r.client.CephFSMkSnapshot(ctx, fs.ID, dirPath, snapName)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Directory '%s' does not exist in '%s'", dirPath, volName),
			)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create snapshot '%s' of '%s' in '%s': %s", snapName, dirPath, volName, err),
		)
		return
	}

	snap, err := r.client.CephFSGetSnapshot(ctx, fs.ID, dirPath, snapName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read snapshot '%s' of '%s' in '%s' after creation: %s", snapName, dirPath, volName, err),
		)
		return
	}

	data.Created = types.StringValue(snap.Created)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CephFSSnapshotResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	fs, err := r.client.CephFSGetByName(ctx, data.VolName.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to look up CephFS filesystem '%s': %s", data.VolName.ValueString(), err),
		)
		return
	}

	snap, err := r.client.CephFSGetSnapshot(ctx, fs.ID, data.Path.ValueString(), data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read snapshot '%s' of '%s' in '%s': %s", data.Name.ValueString(), data.Path.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	data.Created = types.StringValue(snap.Created)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSSnapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update Not Supported",
		"CephFS snapshots are immutable and cannot be updated. Any changes require replacing the resource.",
	)
}

func (r *CephFSSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CephFSSnapshotResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	fs, err := r.client.CephFSGetByName(ctx, data.VolName.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to look up CephFS filesystem '%s': %s", data.VolName.ValueString(), err),
		)
		return
	}

	err = r.client.CephFSRmSnapshot(ctx, fs.ID, data.Path.ValueString(), data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete snapshot '%s' of '%s' in '%s': %s", data.Name.ValueString(), data.Path.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}
}

func (r *CephFSSnapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// vol_name and name cannot contain ':', so the first and last colons
	// bound them and the path may contain colons itself.
	first := strings.Index(req.ID, ":")
	last := strings.LastIndex(req.ID, ":")
	if first <= 0 || last <= first+1 || last == len(req.ID)-1 || !strings.HasPrefix(req.ID[first+1:last], "/") {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: vol_name:/path:snapshot_name, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vol_name"), req.ID[:first])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), req.ID[first+1:last])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID[last+1:])...)
}
