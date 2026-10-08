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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/keyring"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &AuthResource{}
	_ resource.ResourceWithImportState = &AuthResource{}
)

func newAuthResource() resource.Resource {
	return &AuthResource{}
}

type AuthResource struct {
	client *restapi.Client
}

type AuthResourceModel struct {
	Entity  types.String `tfsdk:"entity"`
	Caps    types.Map    `tfsdk:"caps"`
	Key     types.String `tfsdk:"key"`
	KeyType types.String `tfsdk:"key_type"`
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
				Validators: []validator.Map{
					mapvalidator.KeysAre(stringvalidator.OneOf("mds", "mgr", "mon", "osd")),
				},
			},
			"key": resourceSchema.StringAttribute{
				MarkdownDescription: "The cephx key of the entity. If not specified, Ceph will generate a random key. Conflicts with `key_type`.",
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("key_type")),
				},
			},
			"key_type": resourceSchema.StringAttribute{
				MarkdownDescription: "The cephx key type: `aes` (legacy) or `aes256k` (Ceph 19.2.6+/20.2.4+). If not specified, Ceph picks its preferred cipher. Changing it generates a new key in place.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(keyring.KeyTypeAES, keyring.KeyTypeAES256K),
				},
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
	if key == "" && !data.KeyType.IsNull() && !data.KeyType.IsUnknown() {
		key, err = keyring.GenerateKey(data.KeyType.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Key Generation Error", err.Error())
			return
		}
	}
	if key != "" {
		users := []keyring.User{
			{
				Entity: entity,
				Key:    key,
				Caps:   caps,
			},
		}
		importData := keyring.Format(users)
		err = r.client.ClusterImportUser(ctx, importData)
	} else {
		err = r.client.ClusterCreateUser(ctx, entity, caps)
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create user in Ceph API: %s%s", err, keyTypeHint(key)),
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
	keyringRaw, err := r.client.ClusterExportUser(ctx, entity)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to export user from Ceph API: %s", err),
		)
		return
	}

	updateAuthModelFromKeyring(ctx, keyringRaw, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuthResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AuthResourceModel
	var state AuthResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	entity := data.Entity.ValueString()

	caps, ok := mapAttrToCephCaps(ctx, data.Caps, &resp.Diagnostics)
	if !ok {
		return
	}

	var err error
	key := data.Key.ValueString()
	explicitKey := !data.Key.IsNull() && !data.Key.IsUnknown() && key != "" && key != state.Key.ValueString()
	// key_type is unknown in the plan on a caps-only change when it is not
	// configured, which must not count as a change.
	rotate := !explicitKey && !data.KeyType.IsUnknown() && !data.KeyType.Equal(state.KeyType)
	if rotate {
		key, err = keyring.GenerateKey(data.KeyType.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Key Generation Error", err.Error())
			return
		}
	}
	if explicitKey || rotate {
		// ceph auth import replaces both the key and the caps of an
		// existing entity; a plain caps update can never change the key.
		users := []keyring.User{
			{
				Entity: entity,
				Key:    key,
				Caps:   caps,
			},
		}
		err = r.client.ClusterImportUser(ctx, keyring.Format(users))
	} else {
		err = r.client.ClusterUpdateUser(ctx, entity, caps)
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update user in Ceph API: %s%s", err, keyTypeHint(key)),
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
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete user from Ceph API: %s", err),
		)
		return
	}
}

func (r *AuthResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("entity"), req, resp)
}

func updateAuthModelFromCephExport(ctx context.Context, client *restapi.Client, entity string, data *AuthResourceModel, diagnostics *diag.Diagnostics) {
	keyringRaw, err := client.ClusterExportUser(ctx, entity)
	if err != nil {
		diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to export user from Ceph API: %s", err),
		)
		return
	}

	updateAuthModelFromKeyring(ctx, keyringRaw, data, diagnostics)
}

func updateAuthModelFromKeyring(ctx context.Context, keyringRaw string, data *AuthResourceModel, diagnostics *diag.Diagnostics) {
	keyringUsers, err := keyring.Parse(keyringRaw)
	if err != nil {
		diagnostics.AddError(
			"Unable to parse keyring data",
			fmt.Sprintf("Unable to parse keyring data: %s", err),
		)
		return
	} else if len(keyringUsers) == 0 {
		diagnostics.AddError(
			"Empty keyring data",
			"Ceph export returned no users",
		)
		return
	} else if len(keyringUsers) > 1 {
		diagnostics.AddWarning(
			"Ceph export returned multiple users",
			fmt.Sprintf("Ceph export returned multiple users: %s", keyringRaw),
		)
	}
	keyringUser := keyringUsers[0]

	keyType, err := keyring.KeyType(keyringUser.Key)
	if err != nil {
		diagnostics.AddError("Unable to decode key type", err.Error())
		return
	}

	data.Caps = cephCapsToMapValue(ctx, keyringUser.Caps, diagnostics)
	data.Key = types.StringValue(keyringUser.Key)
	data.KeyType = types.StringValue(keyType)
	data.Keyring = types.StringValue(keyringRaw)
}

func keyTypeHint(key string) string {
	if keyType, err := keyring.KeyType(key); err == nil && keyType == keyring.KeyTypeAES256K {
		return " (aes256k keys require Ceph 19.2.6 or 20.2.4 and newer)"
	}
	return ""
}

func mapAttrToCephCaps(ctx context.Context, caps types.Map, diags *diag.Diagnostics) (keyring.Caps, bool) {
	if caps.IsUnknown() {
		diags.AddError("Invalid Capabilities", "caps must be known")
		return keyring.Caps{}, false
	}

	if caps.IsNull() {
		diags.AddError("Invalid Capabilities", "caps must be provided")
		return keyring.Caps{}, false
	}

	var raw map[string]string
	diags.Append(caps.ElementsAs(ctx, &raw, false)...)
	if diags.HasError() {
		return keyring.Caps{}, false
	}

	result, err := keyring.CapsFromMap(raw)
	if err != nil {
		diags.AddError("Invalid Capabilities", err.Error())
		return keyring.Caps{}, false
	}

	return result, true
}
