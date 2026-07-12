package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                = &DashboardRoleResource{}
	_ resource.ResourceWithImportState = &DashboardRoleResource{}
)

func newDashboardRoleResource() resource.Resource {
	return &DashboardRoleResource{}
}

type DashboardRoleResource struct {
	client *restapi.Client
}

type DashboardRoleResourceModel struct {
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	ScopesPermissions types.Map    `tfsdk:"scopes_permissions"`
}

func (r *DashboardRoleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard_role"
}

func (r *DashboardRoleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a custom Ceph dashboard role. Built-in system roles such as `administrator` and `read-only` cannot be created, updated or deleted; use the data source to read them.",
		Attributes: map[string]resourceSchema.Attribute{
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the role. Changing requires destroying and recreating the role.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": resourceSchema.StringAttribute{
				MarkdownDescription: "The description of the role.",
				Optional:            true,
			},
			"scopes_permissions": resourceSchema.MapAttribute{
				MarkdownDescription: "The permissions granted per security scope, as a map from scope name (e.g. `pool`, `rbd-image`, `cephfs`, `rgw`; the set of valid scopes depends on the Ceph release) to a non-empty set of `read`, `create`, `update` and `delete`.",
				Optional:            true,
				ElementType:         types.SetType{ElemType: types.StringType},
				Validators: []validator.Map{
					mapvalidator.ValueSetsAre(
						setvalidator.SizeAtLeast(1),
						setvalidator.ValueStringsAre(
							stringvalidator.OneOf("read", "create", "update", "delete"),
						),
					),
				},
			},
		},
	}
}

func (r *DashboardRoleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DashboardRoleResource) updateModelFromAPI(ctx context.Context, data *DashboardRoleResourceModel, role *restapi.DashboardRole) diag.Diagnostics {
	var diags diag.Diagnostics

	data.Description = types.StringPointerValue(role.Description)

	if data.ScopesPermissions.IsNull() && len(role.ScopesPermissions) == 0 {
		return diags
	}
	scopesPermissions, d := types.MapValueFrom(ctx, types.SetType{ElemType: types.StringType}, role.ScopesPermissions)
	diags.Append(d...)
	data.ScopesPermissions = scopesPermissions

	return diags
}

func (r *DashboardRoleResource) scopesPermissionsFromModel(ctx context.Context, data *DashboardRoleResourceModel, diags *diag.Diagnostics) map[string][]string {
	scopesPermissions := map[string][]string{}
	if !data.ScopesPermissions.IsNull() && !data.ScopesPermissions.IsUnknown() {
		diags.Append(data.ScopesPermissions.ElementsAs(ctx, &scopesPermissions, false)...)
	}
	return scopesPermissions
}

func (r *DashboardRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DashboardRoleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createReq := restapi.DashboardRoleCreateRequest{
		Name:              data.Name.ValueString(),
		Description:       data.Description.ValueStringPointer(),
		ScopesPermissions: r.scopesPermissionsFromModel(ctx, &data, &resp.Diagnostics),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	role, err := r.client.CreateDashboardRole(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create dashboard role '%s': %s", data.Name.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, role)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DashboardRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DashboardRoleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	role, err := r.client.GetDashboardRole(ctx, data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read dashboard role '%s': %s", data.Name.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, role)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DashboardRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DashboardRoleResourceModel
	var state DashboardRoleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := restapi.DashboardRoleUpdateRequest{
		Description:       data.Description.ValueStringPointer(),
		ScopesPermissions: r.scopesPermissionsFromModel(ctx, &data, &resp.Diagnostics),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	role, err := r.client.UpdateDashboardRole(ctx, state.Name.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update dashboard role '%s': %s", state.Name.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, role)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DashboardRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DashboardRoleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteDashboardRole(ctx, data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete dashboard role '%s': %s. Note that a role cannot be deleted while it is assigned to any dashboard user.", data.Name.ValueString(), err),
		)
		return
	}
}

func (r *DashboardRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
