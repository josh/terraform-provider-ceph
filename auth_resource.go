package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &AuthResource{}
	_ resource.ResourceWithImportState = &AuthResource{}
)

func newAuthResource() resource.Resource {
	return &AuthResource{}
}

type AuthResource struct {
	client *CephAPIClient
}

type AuthResourceModel struct {
	Entity  types.String `tfsdk:"entity"`
	Caps    types.Map    `tfsdk:"caps"`
	Key     types.String `tfsdk:"key"`
	Keyring types.String `tfsdk:"keyring"`
}

func (r *AuthResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth"
}

func (r *AuthResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource allows you to manage a ceph client authentication.",
		Attributes: map[string]resourceSchema.Attribute{
			"entity": resourceSchema.StringAttribute{
				MarkdownDescription: "The entity name (i.e.: client.admin)",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"caps": resourceSchema.MapAttribute{
				ElementType:         types.StringType,
				MarkdownDescription: "The caps of the entity",
				Required:            true,
			},
			"key": resourceSchema.StringAttribute{
				MarkdownDescription: "The cephx key of the entity. If not specified, Ceph will generate a random key.",
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
			},
			"keyring": resourceSchema.StringAttribute{
				MarkdownDescription: "The complete cephx keyring as JSON",
				Computed:            true,
				Sensitive:           true,
			},
		},
	}
}

func (r *AuthResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AuthResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AuthResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	entity := data.Entity.ValueString()

	caps, ok := mapAttrToCephCaps(ctx, data.Caps, &resp.Diagnostics)
	if !ok {
		return
	}

	key := data.Key.ValueString()
	var err error
	if key != "" {
		importData := fmt.Sprintf("[%s]\n\tkey = %s\n", entity, key)
		for capEntity, capValue := range caps.Map() {
			importData += fmt.Sprintf("\tcaps %s = \"%s\"\n", capEntity, capValue)
		}
		err = r.client.ClusterImportUser(ctx, importData)
	} else {
		err = r.client.ClusterCreateUser(ctx, entity, caps)
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create user in Ceph API: %s", err),
		)
		return
	}

	updateAuthModelFromCephExport(ctx, r.client, entity, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuthResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AuthResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	entity := data.Entity.ValueString()
	updateAuthModelFromCephExport(ctx, r.client, entity, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuthResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AuthResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	entity := data.Entity.ValueString()

	caps, ok := mapAttrToCephCaps(ctx, data.Caps, &resp.Diagnostics)
	if !ok {
		return
	}

	err := r.client.ClusterUpdateUser(ctx, entity, caps)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update user in Ceph API: %s", err),
		)
		return
	}

	updateAuthModelFromCephExport(ctx, r.client, entity, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuthResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AuthResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	entity := data.Entity.ValueString()
	err := r.client.ClusterDeleteUser(ctx, entity)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete user from Ceph API: %s", err),
		)
		return
	}
}

func (r *AuthResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	data := AuthResourceModel{
		Entity: types.StringValue(req.ID),
	}

	updateAuthModelFromCephExport(ctx, r.client, req.ID, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func updateAuthModelFromCephExport(ctx context.Context, client *CephAPIClient, entity string, data *AuthResourceModel, diagnostics *diag.Diagnostics) {
	keyringRaw, err := client.ClusterExportUser(ctx, entity)
	if err != nil {
		diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to export user from Ceph API: %s", err),
		)
		return
	}

	keyringUsers, err := parseCephKeyring(keyringRaw)
	if err != nil {
		diagnostics.AddError(
			"Unable to parse keyring data",
			fmt.Sprintf("Unable to parse keyring data: %s", err),
		)
		return
	} else if len(keyringUsers) == 0 {
		diagnostics.AddError(
			"Empty keyring data",
			fmt.Sprintf("Ceph export returned no users for entity %s", entity),
		)
		return
	} else if len(keyringUsers) > 1 {
		diagnostics.AddWarning(
			"Ceph export returned multiple users",
			fmt.Sprintf("Ceph export returned multiple users: %s", keyringRaw),
		)
	}
	keyringUser := keyringUsers[0]

	data.Caps = cephCapsToMapValue(ctx, keyringUser.Caps, diagnostics)
	data.Key = types.StringValue(keyringUser.Key)
	data.Keyring = types.StringValue(keyringRaw)
}

func mapAttrToCephCaps(ctx context.Context, caps types.Map, diags *diag.Diagnostics) (CephCaps, bool) {
	if caps.IsUnknown() {
		diags.AddError("Invalid Capabilities", "caps must be known")
		return CephCaps{}, false
	}

	if caps.IsNull() {
		diags.AddError("Invalid Capabilities", "caps must be provided")
		return CephCaps{}, false
	}

	var raw map[string]string
	diags.Append(caps.ElementsAs(ctx, &raw, false)...)
	if diags.HasError() {
		return CephCaps{}, false
	}

	result, err := NewCephCapsFromMap(raw)
	if err != nil {
		diags.AddError("Invalid Capabilities", err.Error())
		return CephCaps{}, false
	}

	return result, true
}

func cephCapsToMapValue(ctx context.Context, caps CephCaps, diags *diag.Diagnostics) types.Map {
	value, err := types.MapValueFrom(ctx, types.StringType, caps.Map())
	if err != nil {
		diags.AddError("State Error", fmt.Sprintf("unable to encode caps: %s", err))
		return types.MapNull(types.StringType)
	}
	return value
}
