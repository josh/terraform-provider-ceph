package main

import (
	"context"
	"errors"
	"fmt"
	gopath "path"
	"strings"

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
	_ resource.Resource                = &CephFSDirectoryResource{}
	_ resource.ResourceWithImportState = &CephFSDirectoryResource{}
)

func newCephFSDirectoryResource() resource.Resource {
	return &CephFSDirectoryResource{}
}

type CephFSDirectoryResource struct {
	client *restapi.Client
}

type CephFSDirectoryResourceModel struct {
	VolName types.String `tfsdk:"vol_name"`
	Path    types.String `tfsdk:"path"`
}

func (r *CephFSDirectoryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs_directory"
}

func (r *CephFSDirectoryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a directory in a CephFS filesystem. A pre-existing directory is adopted. Missing parent directories are created automatically, but only the leaf directory is removed on destroy, and it must be empty.",
		Attributes: map[string]resourceSchema.Attribute{
			"vol_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"path": resourceSchema.StringAttribute{
				MarkdownDescription: "The absolute directory path within the filesystem.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					cephFSDirectoryPath(),
				},
			},
		},
	}
}

func (r *CephFSDirectoryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CephFSDirectoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CephFSDirectoryResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	volName := data.VolName.ValueString()
	dirPath := data.Path.ValueString()

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

	err = r.client.CephFSMkTree(ctx, fs.ID, dirPath)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create directory '%s' in '%s': %s", dirPath, volName, err),
		)
		return
	}

	_, err = r.client.CephFSGetDirectory(ctx, fs.ID, dirPath)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read directory '%s' in '%s' after creation: %s", dirPath, volName, err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSDirectoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CephFSDirectoryResourceModel

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

	_, err = r.client.CephFSGetDirectory(ctx, fs.ID, data.Path.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read directory '%s' in '%s': %s", data.Path.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSDirectoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CephFSDirectoryResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSDirectoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CephFSDirectoryResourceModel

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

	err = r.client.CephFSRmTree(ctx, fs.ID, data.Path.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to remove directory '%s' in '%s': %s", data.Path.ValueString(), data.VolName.ValueString(), err),
		)
		return
	}
}

func (r *CephFSDirectoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || !strings.HasPrefix(parts[1], "/") {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: vol_name:/path, got: %s", req.ID),
		)
		return
	}
	if cleaned := gopath.Clean(parts[1]); cleaned != parts[1] || cleaned == "/" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("The path %q must be normalized (no trailing or repeated slashes, no '.' or '..' segments) and must not be the filesystem root, since the Ceph API reports normalized paths.", parts[1]),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vol_name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), parts[1])...)
}
