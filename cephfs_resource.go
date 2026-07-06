package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &CephFSResource{}
	_ resource.ResourceWithImportState = &CephFSResource{}
)

func newCephFSResource() resource.Resource {
	return &CephFSResource{}
}

type CephFSResource struct {
	client *restapi.Client
}

type CephFSResourceModel struct {
	Name           types.String   `tfsdk:"name"`
	ID             types.Int64    `tfsdk:"id"`
	MetadataPoolID types.Int64    `tfsdk:"metadata_pool_id"`
	DataPoolIDs    types.List     `tfsdk:"data_pool_ids"`
	Timeouts       timeouts.Value `tfsdk:"timeouts"`
}

func (r *CephFSResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cephfs"
}

func (r *CephFSResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a CephFS filesystem. Pools are created automatically.",
		Attributes: map[string]resourceSchema.Attribute{
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CephFS filesystem. Changing the name requires destroying and recreating the filesystem.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": resourceSchema.Int64Attribute{
				MarkdownDescription: "The ID of the CephFS filesystem.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"metadata_pool_id": resourceSchema.Int64Attribute{
				MarkdownDescription: "The ID of the metadata pool (created automatically).",
				Computed:            true,
			},
			"data_pool_ids": resourceSchema.ListAttribute{
				MarkdownDescription: "The IDs of the data pools (created automatically).",
				Computed:            true,
				ElementType:         types.Int64Type,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
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

func (r *CephFSResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CephFSResource) updateModelFromAPI(ctx context.Context, data *CephFSResourceModel, fs *restapi.CephFS) diag.Diagnostics {
	var diags diag.Diagnostics

	data.Name = types.StringValue(fs.Name)
	data.ID = types.Int64Value(int64(fs.ID))
	data.MetadataPoolID = types.Int64Value(int64(fs.MetadataPoolID))

	if len(fs.DataPoolIDs) > 0 {
		poolIDs := make([]int64, len(fs.DataPoolIDs))
		for i, id := range fs.DataPoolIDs {
			poolIDs[i] = int64(id)
		}
		dataPoolIDs, d := types.ListValueFrom(ctx, types.Int64Type, poolIDs)
		diags.Append(d...)
		data.DataPoolIDs = dataPoolIDs
	} else {
		data.DataPoolIDs = types.ListNull(types.Int64Type)
	}

	return diags
}

func (r *CephFSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CephFSResourceModel

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

	createReq := restapi.CephFSCreateRequest{
		Name: data.Name.ValueString(),
		ServiceSpec: map[string]interface{}{
			"placement": map[string]interface{}{},
		},
	}

	tflog.Debug(ctx, "Creating CephFS filesystem", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	err := r.client.CephFSCreate(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create CephFS filesystem '%s': %s", data.Name.ValueString(), err),
		)
		return
	}

	var fs *restapi.CephFS
	for {
		fs, err = r.client.CephFSGetByName(ctx, data.Name.ValueString())
		if err == nil {
			break
		}
		if !errors.Is(err, restapi.ErrNotFound) {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to read CephFS filesystem '%s' after creation: %s", data.Name.ValueString(), err),
			)
			return
		}
		tflog.Debug(ctx, "Waiting for CephFS filesystem to be ready", map[string]interface{}{
			"name": data.Name.ValueString(),
		})
		select {
		case <-ctx.Done():
			resp.Diagnostics.AddError(
				"Timeout",
				fmt.Sprintf("Timeout waiting for CephFS filesystem '%s' to be ready", data.Name.ValueString()),
			)
			return
		case <-time.After(2 * time.Second):
		}
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, fs)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CephFSResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading CephFS filesystem", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	fsName := data.Name.ValueString()

	fs, err := r.client.CephFSGetByName(ctx, fsName)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			tflog.Debug(ctx, "CephFS filesystem not found, removing from state", map[string]interface{}{
				"fs_name": fsName,
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read CephFS filesystem '%s': %s", fsName, err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, fs)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CephFSResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	fsName := data.Name.ValueString()

	fs, err := r.client.CephFSGetByName(ctx, fsName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read CephFS filesystem '%s': %s", fsName, err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, fs)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CephFSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CephFSResourceModel

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

	fsName := data.Name.ValueString()

	tflog.Debug(ctx, "Deleting CephFS filesystem", map[string]interface{}{
		"name": fsName,
	})

	err := r.client.CephFSDelete(ctx, fsName)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete CephFS filesystem '%s': %s", fsName, err),
		)
		return
	}
}

func (r *CephFSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
