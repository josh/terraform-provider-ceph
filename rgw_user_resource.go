package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &RGWUserResource{}
	_ resource.ResourceWithImportState = &RGWUserResource{}
)

func newRGWUserResource() resource.Resource {
	return &RGWUserResource{}
}

type RGWUserResource struct {
	client *CephAPIClient
}

type RGWUserResourceModel struct {
	UID         types.String `tfsdk:"uid"`
	DisplayName types.String `tfsdk:"display_name"`
	Email       types.String `tfsdk:"email"`
	MaxBuckets  types.Int64  `tfsdk:"max_buckets"`
	System      types.Bool   `tfsdk:"system"`
	Suspended   types.Bool   `tfsdk:"suspended"`
	Tenant      types.String `tfsdk:"tenant"`
	Admin       types.Bool   `tfsdk:"admin"`
	UserID      types.String `tfsdk:"user_id"`
}

func (r *RGWUserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_user"
}

func (r *RGWUserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource allows you to manage a Ceph RGW user.",
		Attributes: map[string]resourceSchema.Attribute{
			"uid": resourceSchema.StringAttribute{
				MarkdownDescription: "The user ID",
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
			},
			"suspended": resourceSchema.BoolAttribute{
				MarkdownDescription: "Whether this user is suspended",
				Optional:            true,
				Computed:            true,
			},
			"tenant": resourceSchema.StringAttribute{
				MarkdownDescription: "Tenant for multi-tenancy support",
				Computed:            true,
			},
			"admin": resourceSchema.BoolAttribute{
				MarkdownDescription: "Whether this user has admin privileges (read-only, can only be set via radosgw-admin CLI)",
				Computed:            true,
			},
			"user_id": resourceSchema.StringAttribute{
				MarkdownDescription: "The user ID returned by the API",
				Computed:            true,
			},
		},
	}
}

func (r *RGWUserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RGWUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RGWUserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createReq := CephAPIRGWUserCreateRequest{
		UID:         data.UID.ValueString(),
		DisplayName: data.DisplayName.ValueString(),
	}

	if !data.Email.IsNull() && !data.Email.IsUnknown() {
		createReq.Email = data.Email.ValueString()
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

	updateModelFromAPIUser(ctx, &data, user)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RGWUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	uid := data.UID.ValueString()
	user, err := r.client.RGWGetUser(ctx, uid)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW user: %s", err),
		)
		return
	}

	updateModelFromAPIUser(ctx, &data, user)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RGWUserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	uid := data.UID.ValueString()
	updateReq := CephAPIRGWUserUpdateRequest{}

	if !data.DisplayName.IsNull() && !data.DisplayName.IsUnknown() {
		displayName := data.DisplayName.ValueString()
		updateReq.DisplayName = &displayName
	}

	if !data.Email.IsNull() && !data.Email.IsUnknown() {
		email := data.Email.ValueString()
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

	if !data.Suspended.IsNull() && !data.Suspended.IsUnknown() {
		suspended := 0
		if data.Suspended.ValueBool() {
			suspended = 1
		}
		updateReq.Suspended = &suspended
	}

	user, err := r.client.RGWUpdateUser(ctx, uid, updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update RGW user: %s", err),
		)
		return
	}

	updateModelFromAPIUser(ctx, &data, user)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RGWUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	uid := data.UID.ValueString()
	err := r.client.RGWDeleteUser(ctx, uid)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete RGW user: %s", err),
		)
		return
	}
}

func (r *RGWUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	uid := req.ID

	user, err := r.client.RGWGetUser(ctx, uid)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW user during import: %s", err),
		)
		return
	}

	data := RGWUserResourceModel{
		UID: types.StringValue(uid),
	}
	updateModelFromAPIUser(ctx, &data, user)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func updateModelFromAPIUser(ctx context.Context, data *RGWUserResourceModel, user CephAPIRGWUser) {
	data.UserID = types.StringValue(user.UserID)
	data.DisplayName = types.StringValue(user.DisplayName)
	data.MaxBuckets = types.Int64Value(int64(user.MaxBuckets))
	data.System = types.BoolValue(user.System)
	data.Admin = types.BoolValue(user.Admin)
	data.Suspended = types.BoolValue(user.Suspended == 1)
	data.Tenant = types.StringValue(user.Tenant)
}
