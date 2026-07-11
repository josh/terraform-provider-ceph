package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &RGWRoleResource{}
	_ resource.ResourceWithImportState = &RGWRoleResource{}
)

func newRGWRoleResource() resource.Resource {
	return &RGWRoleResource{}
}

type RGWRoleResource struct {
	client *restapi.Client
}

type RGWRoleResourceModel struct {
	Name                     types.String `tfsdk:"name"`
	Path                     types.String `tfsdk:"path"`
	AssumeRolePolicyDocument types.String `tfsdk:"assume_role_policy_document"`
	MaxSessionDuration       types.Int64  `tfsdk:"max_session_duration"`
	ID                       types.String `tfsdk:"id"`
	RoleID                   types.String `tfsdk:"role_id"`
	Arn                      types.String `tfsdk:"arn"`
	CreateDate               types.String `tfsdk:"create_date"`
	AccountID                types.String `tfsdk:"account_id"`
}

func (r *RGWRoleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_role"
}

func (r *RGWRoleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a Ceph RGW IAM role.",
		Attributes: map[string]resourceSchema.Attribute{
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the role. Changing requires destroying and recreating the role.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(64),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"path": resourceSchema.StringAttribute{
				MarkdownDescription: "The path to the role. Must be an absolute path beginning and ending with a slash. Changing requires destroying and recreating the role.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("/"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"assume_role_policy_document": resourceSchema.StringAttribute{
				MarkdownDescription: "The trust relationship policy document that grants an entity permission to assume the role, as a JSON string. Changing requires destroying and recreating the role.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"max_session_duration": resourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum session duration in seconds for the role. Must be between 3600 (1 hour) and 43200 (12 hours).",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(3600),
				Validators: []validator.Int64{
					int64validator.Between(3600, 43200),
				},
			},
			"id": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the role.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"role_id": resourceSchema.StringAttribute{
				MarkdownDescription: "The internal id of the role.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"arn": resourceSchema.StringAttribute{
				MarkdownDescription: "The Amazon Resource Name (ARN) of the role.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"create_date": resourceSchema.StringAttribute{
				MarkdownDescription: "The date and time the role was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"account_id": resourceSchema.StringAttribute{
				MarkdownDescription: "The account id the role belongs to.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *RGWRoleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RGWRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RGWRoleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	err := r.client.RGWCreateRole(ctx, restapi.RGWRoleCreateRequest{
		RoleName:            name,
		RolePath:            data.Path.ValueString(),
		RoleAssumePolicyDoc: data.AssumeRolePolicyDocument.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create RGW role: %s", err),
		)
		return
	}

	if data.MaxSessionDuration.ValueInt64() != 3600 {
		if err := r.client.RGWUpdateRole(ctx, name, data.MaxSessionDuration.ValueInt64()); err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to set max session duration on RGW role: %s", err),
			)
			return
		}
	}

	role, err := r.client.RGWGetRole(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW role after creation: %s", err),
		)
		return
	}

	updateModelFromAPIRole(&data, role)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RGWRoleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	role, err := r.client.RGWGetRole(ctx, data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW role: %s", err),
		)
		return
	}

	updateModelFromAPIRole(&data, role)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RGWRoleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	err := r.client.RGWUpdateRole(ctx, name, data.MaxSessionDuration.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update RGW role: %s", err),
		)
		return
	}

	role, err := r.client.RGWGetRole(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read RGW role after update: %s", err),
		)
		return
	}

	updateModelFromAPIRole(&data, role)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RGWRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RGWRoleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.RGWDeleteRole(ctx, data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete RGW role: %s", err),
		)
		return
	}
}

func (r *RGWRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func updateModelFromAPIRole(data *RGWRoleResourceModel, role *restapi.RGWRole) {
	data.Name = types.StringValue(role.RoleName)
	data.Path = types.StringValue(role.Path)
	data.MaxSessionDuration = types.Int64Value(role.MaxSessionDuration)
	data.ID = types.StringValue(role.RoleName)
	data.RoleID = types.StringValue(role.RoleID)
	data.Arn = types.StringValue(role.Arn)
	data.CreateDate = types.StringValue(role.CreateDate)
	data.AccountID = types.StringValue(role.AccountID)

	// Preserve the configured document when it is semantically equal to the
	// value returned by Ceph to avoid perpetual diffs from JSON formatting.
	if !jsonStringsEqual(data.AssumeRolePolicyDocument.ValueString(), role.AssumeRolePolicyDocument) {
		data.AssumeRolePolicyDocument = types.StringValue(role.AssumeRolePolicyDocument)
	}
}

func jsonStringsEqual(a, b string) bool {
	var av, bv any
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}
