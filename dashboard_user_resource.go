package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &DashboardUserResource{}
	_ resource.ResourceWithImportState = &DashboardUserResource{}
)

func newDashboardUserResource() resource.Resource {
	return &DashboardUserResource{}
}

type DashboardUserResource struct {
	client *restapi.Client
}

type DashboardUserResourceModel struct {
	Username          types.String `tfsdk:"username"`
	Password          types.String `tfsdk:"password"`
	Name              types.String `tfsdk:"name"`
	Email             types.String `tfsdk:"email"`
	Roles             types.Set    `tfsdk:"roles"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	PwdExpirationDate types.Int64  `tfsdk:"pwd_expiration_date"`
	PwdUpdateRequired types.Bool   `tfsdk:"pwd_update_required"`
}

func (r *DashboardUserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard_user"
}

func (r *DashboardUserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a Ceph dashboard user account. Dashboard users are distinct from cluster (CephX) users, which are managed by `ceph_auth`.",
		Attributes: map[string]resourceSchema.Attribute{
			"username": resourceSchema.StringAttribute{
				MarkdownDescription: "The username of the dashboard user. Changing requires destroying and recreating the user.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": resourceSchema.StringAttribute{
				MarkdownDescription: "The password for the dashboard user. The Ceph API never returns passwords, so out-of-band password changes cannot be detected; changing this value resets the password.",
				Required:            true,
				Sensitive:           true,
			},
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The full name of the user.",
				Optional:            true,
			},
			"email": resourceSchema.StringAttribute{
				MarkdownDescription: "The email address of the user.",
				Optional:            true,
			},
			"roles": resourceSchema.SetAttribute{
				MarkdownDescription: "The roles assigned to the user. Built-in roles are `administrator`, `read-only`, `block-manager`, `rgw-manager`, `cluster-manager`, `pool-manager`, `cephfs-manager` and `ganesha-manager`; custom role names are also accepted.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"enabled": resourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the user account is enabled. Defaults to true.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"pwd_expiration_date": resourceSchema.Int64Attribute{
				MarkdownDescription: "The password expiration date as epoch seconds.",
				Optional:            true,
			},
			"pwd_update_required": resourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the user must change their password before using the dashboard or API. Defaults to false, since a pending password change blocks all API access for the user.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *DashboardUserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DashboardUserResource) updateModelFromAPI(ctx context.Context, data *DashboardUserResourceModel, user *restapi.DashboardUser) diag.Diagnostics {
	var diags diag.Diagnostics

	data.Name = types.StringPointerValue(user.Name)
	data.Email = types.StringPointerValue(user.Email)
	data.Enabled = types.BoolValue(user.Enabled)
	data.PwdExpirationDate = types.Int64PointerValue(user.PwdExpirationDate)
	data.PwdUpdateRequired = types.BoolValue(user.PwdUpdateRequired)

	if data.Roles.IsNull() && len(user.Roles) == 0 {
		return diags
	}
	roles, d := types.SetValueFrom(ctx, types.StringType, user.Roles)
	diags.Append(d...)
	data.Roles = roles

	return diags
}

func (r *DashboardUserResource) rolesFromModel(ctx context.Context, data *DashboardUserResourceModel, diags *diag.Diagnostics) []string {
	roles := []string{}
	if !data.Roles.IsNull() && !data.Roles.IsUnknown() {
		diags.Append(data.Roles.ElementsAs(ctx, &roles, false)...)
	}
	return roles
}

func (r *DashboardUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DashboardUserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createReq := restapi.DashboardUserCreateRequest{
		Username:          data.Username.ValueString(),
		Password:          data.Password.ValueString(),
		Name:              data.Name.ValueStringPointer(),
		Email:             data.Email.ValueStringPointer(),
		Roles:             r.rolesFromModel(ctx, &data, &resp.Diagnostics),
		Enabled:           data.Enabled.ValueBool(),
		PwdExpirationDate: data.PwdExpirationDate.ValueInt64Pointer(),
		PwdUpdateRequired: data.PwdUpdateRequired.ValueBool(),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := r.client.CreateDashboardUser(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create dashboard user '%s': %s", data.Username.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, user)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DashboardUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DashboardUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	user, err := r.client.GetDashboardUser(ctx, data.Username.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read dashboard user '%s': %s", data.Username.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, user)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DashboardUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DashboardUserResourceModel
	var state DashboardUserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := restapi.DashboardUserUpdateRequest{
		Name:              data.Name.ValueStringPointer(),
		Email:             data.Email.ValueStringPointer(),
		Roles:             r.rolesFromModel(ctx, &data, &resp.Diagnostics),
		Enabled:           data.Enabled.ValueBool(),
		PwdExpirationDate: data.PwdExpirationDate.ValueInt64Pointer(),
		PwdUpdateRequired: data.PwdUpdateRequired.ValueBool(),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.Password.Equal(state.Password) {
		updateReq.Password = data.Password.ValueStringPointer()
	}

	user, err := r.client.UpdateDashboardUser(ctx, state.Username.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update dashboard user '%s': %s", state.Username.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, user)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DashboardUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DashboardUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteDashboardUser(ctx, data.Username.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete dashboard user '%s': %s. Note that the user the provider authenticates as cannot be deleted.", data.Username.ValueString(), err),
		)
		return
	}
}

func (r *DashboardUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The API never returns passwords, so password stays null in state and
	// the first apply after import resets it to the configured value.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("username"), req.ID)...)
}
