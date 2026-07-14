package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &RGWUserResource{}
	_ resource.ResourceWithImportState = &RGWUserResource{}
)

func newRGWUserResource() resource.Resource {
	return &RGWUserResource{}
}

type RGWUserResource struct {
	client *restapi.Client
}

type RGWUserResourceModel struct {
	UserID      types.String `tfsdk:"user_id"`
	DisplayName types.String `tfsdk:"display_name"`
	Email       types.String `tfsdk:"email"`
	MaxBuckets  types.Int64  `tfsdk:"max_buckets"`
	System      types.Bool   `tfsdk:"system"`
	Suspended   types.Bool   `tfsdk:"suspended"`
	Caps        types.Map    `tfsdk:"caps"`
	Tenant      types.String `tfsdk:"tenant"`
	Admin       types.Bool   `tfsdk:"admin"`
}

func (r *RGWUserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_user"
}

func (r *RGWUserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource allows you to manage a Ceph RGW user.",
		Attributes: map[string]resourceSchema.Attribute{
			"user_id": resourceSchema.StringAttribute{
				MarkdownDescription: "The user identifier for this RGW user",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The display name of the user",
				Required:            true,
			},
			"email": resourceSchema.StringAttribute{
				MarkdownDescription: "The email address of the user",
				Optional:            true,
			},
			"max_buckets": resourceSchema.Int64Attribute{
				MarkdownDescription: "Maximum number of buckets the user can own",
				Optional:            true,
				Computed:            true,
			},
			"system": resourceSchema.BoolAttribute{
				MarkdownDescription: "Whether this is a system user",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"suspended": resourceSchema.BoolAttribute{
				MarkdownDescription: "Whether this user is suspended",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"caps": resourceSchema.MapAttribute{
				MarkdownDescription: "Administrative capabilities of the user, as a map from capability type (e.g. `usage`, `buckets`, `users`, `metadata`, `zone`) to permission. RGW reports combined read and write permissions as `*`, so only `read`, `write` and `*` are accepted.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Validators: []validator.Map{
					mapvalidator.ValueStringsAre(
						stringvalidator.OneOf("read", "write", "*"),
					),
				},
			},
			"tenant": resourceSchema.StringAttribute{
				MarkdownDescription: "The tenant this user belongs to (empty string for default tenant in multi-tenancy configurations)",
				Computed:            true,
			},
			"admin": resourceSchema.BoolAttribute{
				MarkdownDescription: "Whether this user has admin privileges (can only be set via radosgw-admin CLI)",
				Computed:            true,
			},
		},
	}
}

func (r *RGWUserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RGWUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RGWUserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createReq := restapi.RGWUserCreateRequest{
		UID:         data.UserID.ValueString(),
		DisplayName: data.DisplayName.ValueString(),
	}

	if !data.Email.IsNull() && !data.Email.IsUnknown() {
		email := data.Email.ValueString()
		createReq.Email = &email
	}

	if !data.MaxBuckets.IsNull() && !data.MaxBuckets.IsUnknown() {
		maxBuckets := int(data.MaxBuckets.ValueInt64())
		createReq.MaxBuckets = &maxBuckets
	}

	if !data.System.IsNull() && !data.System.IsUnknown() {
		system := data.System.ValueBool()
		createReq.System = &system
	}

	if !data.Suspended.IsNull() && !data.Suspended.IsUnknown() {
		suspended := 0
		if data.Suspended.ValueBool() {
			suspended = 1
		}
		createReq.Suspended = &suspended
	}

	createReq.GenerateKey = false

	user, err := r.client.RGWCreateUser(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create RGW user: %s", err),
		)
		return
	}

	// The user exists from here on, so record it before applying
	// capabilities to keep a failure there from orphaning it from state.
	partial := data
	resp.Diagnostics.Append(updateModelFromAPIUser(ctx, &partial, user)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &partial)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.Caps.IsNull() && !data.Caps.IsUnknown() {
		var caps map[string]string
		resp.Diagnostics.Append(data.Caps.ElementsAs(ctx, &caps, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for capType, perm := range caps {
			if err := r.client.RGWCreateUserCap(ctx, data.UserID.ValueString(), capType, perm); err != nil {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to add capability '%s=%s' to RGW user: %s", capType, perm, err),
				)
				return
			}
		}
		user, err = r.client.RGWGetUser(ctx, data.UserID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to read RGW user after adding capabilities: %s", err),
			)
			return
		}
	}

	resp.Diagnostics.Append(updateModelFromAPIUser(ctx, &data, user)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RGWUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	userID := data.UserID.ValueString()
	user, err := r.client.RGWGetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW user: %s", err),
		)
		return
	}

	resp.Diagnostics.Append(updateModelFromAPIUser(ctx, &data, user)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RGWUserResourceModel
	var state RGWUserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	userID := data.UserID.ValueString()
	updateReq := restapi.RGWUserUpdateRequest{}

	if !data.DisplayName.IsNull() && !data.DisplayName.IsUnknown() {
		displayName := data.DisplayName.ValueString()
		updateReq.DisplayName = &displayName
	}

	if !data.Email.IsNull() && !data.Email.IsUnknown() {
		email := data.Email.ValueString()
		updateReq.Email = &email
	} else if data.Email.IsNull() && !state.Email.IsNull() {
		email := ""
		updateReq.Email = &email
	}

	if !data.MaxBuckets.IsNull() && !data.MaxBuckets.IsUnknown() {
		maxBuckets := int(data.MaxBuckets.ValueInt64())
		updateReq.MaxBuckets = &maxBuckets
	}

	if !data.System.IsNull() && !data.System.IsUnknown() {
		system := data.System.ValueBool()
		updateReq.System = &system
	}

	if !data.Suspended.IsNull() && !data.Suspended.IsUnknown() {
		suspended := 0
		if data.Suspended.ValueBool() {
			suspended = 1
		}
		updateReq.Suspended = &suspended
	}

	user, err := r.client.RGWUpdateUser(ctx, userID, updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update RGW user: %s", err),
		)
		return
	}

	if !data.Caps.Equal(state.Caps) {
		var planCaps, stateCaps map[string]string
		if !data.Caps.IsNull() && !data.Caps.IsUnknown() {
			resp.Diagnostics.Append(data.Caps.ElementsAs(ctx, &planCaps, false)...)
		}
		if !state.Caps.IsNull() {
			resp.Diagnostics.Append(state.Caps.ElementsAs(ctx, &stateCaps, false)...)
		}
		if resp.Diagnostics.HasError() {
			return
		}

		// A changed permission is a delete of the old capability followed
		// by an add, since adding merges permission bits.
		for capType, perm := range stateCaps {
			if planPerm, ok := planCaps[capType]; ok && planPerm == perm {
				continue
			}
			if err := r.client.RGWDeleteUserCap(ctx, userID, capType, perm); err != nil && !errors.Is(err, restapi.ErrNotFound) {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to remove capability '%s=%s' from RGW user: %s", capType, perm, err),
				)
				return
			}
		}
		for capType, perm := range planCaps {
			if statePerm, ok := stateCaps[capType]; ok && statePerm == perm {
				continue
			}
			if err := r.client.RGWCreateUserCap(ctx, userID, capType, perm); err != nil {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to add capability '%s=%s' to RGW user: %s", capType, perm, err),
				)
				return
			}
		}

		user, err = r.client.RGWGetUser(ctx, userID)
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to read RGW user after updating capabilities: %s", err),
			)
			return
		}
	}

	resp.Diagnostics.Append(updateModelFromAPIUser(ctx, &data, user)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RGWUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	userID := data.UserID.ValueString()
	err := r.client.RGWDeleteUser(ctx, userID)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete RGW user: %s", err),
		)
		return
	}
}

func (r *RGWUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("user_id"), req, resp)
}

func updateModelFromAPIUser(ctx context.Context, data *RGWUserResourceModel, user *restapi.RGWUser) diag.Diagnostics {
	var diags diag.Diagnostics

	if user.Tenant != "" {
		data.UserID = types.StringValue(user.Tenant + "$" + user.UserID)
	} else {
		data.UserID = types.StringValue(user.UserID)
	}
	data.DisplayName = types.StringValue(user.DisplayName)
	switch {
	case user.Email != "":
		data.Email = types.StringValue(user.Email)
	case !data.Email.IsNull() && !data.Email.IsUnknown():
		data.Email = types.StringValue("")
	default:
		data.Email = types.StringNull()
	}
	data.MaxBuckets = types.Int64Value(int64(user.MaxBuckets))
	data.System = types.BoolValue(user.System)
	data.Admin = types.BoolValue(user.Admin)
	data.Suspended = types.BoolValue(user.Suspended == 1)
	data.Tenant = types.StringValue(user.Tenant)

	if !data.Caps.IsNull() || len(user.Caps) > 0 {
		capsMap := make(map[string]string, len(user.Caps))
		for _, c := range user.Caps {
			capsMap[c.Type] = c.Perm
		}
		caps, d := types.MapValueFrom(ctx, types.StringType, capsMap)
		diags.Append(d...)
		data.Caps = caps
	}

	return diags
}
