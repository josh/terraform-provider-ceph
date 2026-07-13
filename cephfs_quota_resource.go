package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &CephFSQuotaResource{}
	_ resource.ResourceWithImportState = &CephFSQuotaResource{}
)

func newCephFSQuotaResource() resource.Resource {
	return &CephFSQuotaResource{}
}

type CephFSQuotaResource struct {
	client *restapi.Client
}

type CephFSQuotaResourceModel struct {
	VolName  types.String `tfsdk:"vol_name"`
	Path     types.String `tfsdk:"path"`
	MaxBytes types.Int64  `tfsdk:"max_bytes"`
	MaxFiles types.Int64  `tfsdk:"max_files"`
}

func (r *CephFSQuotaResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs_quota"
}

func (r *CephFSQuotaResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages the quota of a CephFS directory. Every directory always has quota settings, where 0 means unlimited; creating this resource sets them and destroying it resets both limits to 0.",
		Attributes: map[string]resourceSchema.Attribute{
			"vol_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"path": resourceSchema.StringAttribute{
				MarkdownDescription: "The directory path within the filesystem.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"max_bytes": resourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum number of bytes allowed under the path. 0 means unlimited.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"max_files": resourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum number of files allowed under the path. 0 means unlimited.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
		},
	}
}

func (r *CephFSQuotaResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CephFSQuotaResource) setAndRead(ctx context.Context, data *CephFSQuotaResourceModel) error {
	volName := data.VolName.ValueString()
	fsPath := data.Path.ValueString()

	fs, err := r.client.CephFSGetByName(ctx, volName)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return fmt.Errorf("CephFS filesystem '%s' does not exist", volName)
		}
		return fmt.Errorf("unable to look up CephFS filesystem '%s': %w", volName, err)
	}

	err = r.client.SetCephFSQuota(ctx, fs.ID, fsPath, data.MaxBytes.ValueInt64(), data.MaxFiles.ValueInt64())
	if err != nil {
		return fmt.Errorf("unable to set quota on '%s' in '%s': %w", fsPath, volName, err)
	}

	quota, err := r.client.GetCephFSQuota(ctx, fs.ID, fsPath)
	if err != nil {
		return fmt.Errorf("unable to read quota on '%s' in '%s' after setting it: %w", fsPath, volName, err)
	}

	data.MaxBytes = types.Int64Value(quota.MaxBytes)
	data.MaxFiles = types.Int64Value(quota.MaxFiles)
	return nil
}

func (r *CephFSQuotaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CephFSQuotaResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setAndRead(ctx, &data); err != nil {
		resp.Diagnostics.AddError("API Request Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSQuotaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CephFSQuotaResourceModel

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

	quota, err := r.client.GetCephFSQuota(ctx, fs.ID, data.Path.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read quota on '%s' in '%s': %s", data.Path.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	data.MaxBytes = types.Int64Value(quota.MaxBytes)
	data.MaxFiles = types.Int64Value(quota.MaxFiles)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSQuotaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CephFSQuotaResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setAndRead(ctx, &data); err != nil {
		resp.Diagnostics.AddError("API Request Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSQuotaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CephFSQuotaResourceModel

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

	err = r.client.SetCephFSQuota(ctx, fs.ID, data.Path.ValueString(), 0, 0)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to reset quota on '%s' in '%s': %s", data.Path.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}
}

func (r *CephFSQuotaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || !strings.HasPrefix(parts[1], "/") {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: vol_name:/path, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vol_name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), parts[1])...)
}
